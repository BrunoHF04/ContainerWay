package webapp

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// OpenBrowser abre a URL no navegador predefinido do sistema.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// tryReuseRunningInstance verifica se o serviço já está ativo; se sim, abre o browser e indica reutilização.
func tryReuseRunningInstance(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/api/health")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	_ = OpenBrowser(baseURL)
	return true
}

// isAddrInUse indica se o erro de Listen é porta/endereço já ocupado.
func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "in use") ||
		strings.Contains(strings.ToLower(err.Error()), "bind") ||
		strings.Contains(strings.ToLower(err.Error()), "já está em uso") ||
		strings.Contains(strings.ToLower(err.Error()), "only one usage")
}

// formatBaseURL monta URL http a partir do endereço de escuta.
func formatBaseURL(host, port string) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		// host já é host:port em alguns casos
		return "http://" + host
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}
