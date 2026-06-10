package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeployGenerateRequest dados enviados para gerar a chave da VPN.
type DeployGenerateRequest struct {
	Name string `json:"name"`
}

// generateHeadscaleKey faz a chamada na API da sua VPS Headscale para gerar uma PreAuthKey.
func generateHeadscaleKey(hsURL, apiKey, userName string) (string, error) {
	if hsURL == "" || apiKey == "" {
		return "", fmt.Errorf("Headscale não configurado. Forneça o endereço da VPS (-hs-url) e a chave de API (-hs-apikey)")
	}

	// Normaliza URL do servidor Headscale
	apiURL := strings.TrimRight(hsURL, "/") + "/api/v1/preauthkey"

	// Define expiração em 24 horas
	expiration := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	// Payload oficial da API do Headscale
	bodyData := map[string]any{
		"user":       userName,
		"reusable":   false,
		"expiration": expiration,
		"ephemeral":  false,
	}

	jsonBytes, err := json.Marshal(bodyData)
	if err != nil {
		return "", fmt.Errorf("falha ao serializar payload do Headscale: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("falha ao criar requisição para Headscale: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("falha na conexão HTTP com a VPS Headscale: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyErr, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("VPS Headscale retornou erro (%d): %s", resp.StatusCode, string(bodyErr))
	}

	// Estrutura de resposta da API do Headscale
	var respData struct {
		Key struct {
			Key string `json:"key"`
		} `json:"key"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", fmt.Errorf("falha ao decodificar resposta da VPS Headscale: %w", err)
	}

	if respData.Key.Key == "" {
		return "", fmt.Errorf("a VPS Headscale não retornou uma chave de autenticação válida")
	}

	return respData.Key.Key, nil
}

// handleDeployGenerate manipula a geração da chave no Headscale e retorna o comando curl formatado.
func (s *Server) handleDeployGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}

	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}

	var req DeployGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}

	clientName := strings.TrimSpace(req.Name)
	if clientName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "o nome do cliente é obrigatório"})
		return
	}

	// Gera a chave no Headscale
	vpnKey, err := generateHeadscaleKey(s.opts.HSURL, s.opts.HSApiKey, "clientes")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Define os endpoints dinâmicos usando o Host da requisição
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	scriptURL := fmt.Sprintf("%s://%s/api/deploy/install.sh", scheme, host)
	downloadURL := fmt.Sprintf("%s://%s/api/deploy/download", scheme, host)

	// Usa o endereço de controle da VPN configurado, ou cai de volta para a URL do Headscale
	vpnServer := s.opts.TSControlURL
	if vpnServer == "" {
		vpnServer = s.opts.HSURL
	}

	// Monta o comando de instalação final
	cmd := fmt.Sprintf("curl -fsSL %s | sudo sh -s -- -key %q -name %q -server %q -download %q",
		scriptURL, vpnKey, clientName, vpnServer, downloadURL)

	writeJSON(w, http.StatusOK, map[string]string{
		"client":      clientName,
		"vpnKey":      vpnKey,
		"command":     cmd,
		"vpnServer":   vpnServer,
		"downloadUrl": downloadURL,
	})
}

// handleDeployInstallScript devolve o script bash autoinstalador para o cliente Linux.
func (s *Server) handleDeployInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")

	script := `#!/bin/sh
# Script de Instalação Automática do ContainerWay Web Agent

VPN_KEY=""
CLIENT_NAME=""
VPN_SERVER=""
DOWNLOAD_URL=""

while [ $# -gt 0 ]; do
  case "$1" in
    -key) VPN_KEY="$2"; shift 2;;
    -name) CLIENT_NAME="$2"; shift 2;;
    -server) VPN_SERVER="$2"; shift 2;;
    -download) DOWNLOAD_URL="$2"; shift 2;;
    *) shift;;
  esac
done

if [ -z "$VPN_KEY" ] || [ -z "$CLIENT_NAME" ] || [ -z "$VPN_SERVER" ] || [ -z "$DOWNLOAD_URL" ]; then
  echo "Erro: Parâmetros obrigatórios ausentes."
  echo "Uso: install.sh -key <key> -name <name> -server <server> -download <download_url>"
  exit 1
fi

echo "=========================================="
echo " Iniciando Instalação do ContainerWay Web Agent"
echo "=========================================="

# 1. Validar arquitetura
ARCH=$(uname -m)
if [ "$ARCH" != "x86_64" ] && [ "$ARCH" != "amd64" ]; then
  echo "Erro: Arquitetura $ARCH não suportada. Apenas amd64 / x86_64 são suportados."
  exit 1
fi

# 2. Criar diretório destino se não existir
mkdir -p /usr/local/bin

# 3. Baixar binário
echo "Baixando o agente Linux de $DOWNLOAD_URL..."
curl -fsSL -o /usr/local/bin/containerway-web "$DOWNLOAD_URL"
if [ $? -ne 0 ]; then
  echo "Erro ao baixar o binário. Verifique a conexão com o servidor."
  exit 1
fi
chmod +x /usr/local/bin/containerway-web

# 4. Criar serviço systemd
echo "Configurando serviço em segundo plano (systemd)..."
cat <<EOF > /etc/systemd/system/containerway-web.service
[Unit]
Description=ContainerWay Web Agent VPN
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/containerway-web -ts-authkey="$VPN_KEY" -ts-hostname="$CLIENT_NAME" -ts-control-url="$VPN_SERVER" -no-browser=true -auto-shutdown=false
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# 5. Ativar e reiniciar o serviço
echo "Iniciando o serviço..."
systemctl daemon-reload
systemctl enable containerway-web
systemctl restart containerway-web

if [ $? -eq 0 ]; then
  echo "=========================================="
  echo " Instalação finalizada com sucesso!"
  echo " O agente está conectando na VPN do Headscale."
  echo "=========================================="
else
  echo "Erro ao iniciar o serviço no systemd."
  exit 1
fi
`
	_, _ = w.Write([]byte(script))
}

// handleDeployDownload envia o binário Linux para download do cliente.
func (s *Server) handleDeployDownload(w http.ResponseWriter, r *http.Request) {
	// Procura o executável na pasta dist/linux/
	binPath := filepath.Join(".", "dist", "linux", "containerway-web")
	if envPath := os.Getenv("CONTAINERWAY_LINUX_BIN"); envPath != "" {
		binPath = envPath
	}

	file, err := os.Open(binPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "O binário do agente Linux não foi encontrado no servidor. Compile-o primeiro usando: scripts/build.ps1 -SkipWindows",
		})
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", "attachment; filename=containerway-web")
	w.Header().Set("Content-Type", "application/octet-stream")

	_, _ = io.Copy(w, file)
}
