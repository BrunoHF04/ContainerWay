// Package automation regras e motor de execução partilhados (desktop e web).
package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

const (
	// KindDockerStoppedRestart reinicia contêiner Docker quando deixa de correr.
	KindDockerStoppedRestart = "docker_container_stopped_restart"
)

// Rule descreve uma regra de automação persistida por host.
type Rule struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Trigger     string `json:"trigger"`
	Action      string `json:"action"`
	Target      string `json:"target"`
	CooldownSec int    `json:"cooldownSec"`
	Enabled     bool   `json:"enabled"`
	WebhookURL  string `json:"webhookURL,omitempty"`
}

// DefaultRules devolve regras iniciais quando não existe ficheiro.
func DefaultRules() []Rule {
	return []Rule{
		{
			ID:          "auto-restart-critico",
			Kind:        KindDockerStoppedRestart,
			Name:        "Reinício automático (crítico)",
			Description: "Reinicia contêineres parados configurados como alvo.",
			Trigger:     "Contêiner Docker deixou de correr",
			Action:      "docker restart",
			Target:      "",
			CooldownSec: 20,
			Enabled:     true,
		},
		{
			ID:          "protecao-disco",
			Kind:        "placeholder_disk_cleanup",
			Name:        "Proteção de disco (em breve)",
			Description: "Limpeza automática quando o disco enche.",
			Trigger:     "Uso de disco acima do limite",
			Action:      "Limpar arquivos temporários",
			Target:      "/var",
			CooldownSec: 60,
			Enabled:     false,
		},
		{
			ID:          "diagnostico-pos-erro",
			Kind:        "placeholder_diagnostic_bundle",
			Name:        "Diagnóstico pós-erro (em breve)",
			Description: "Recolhe informação de diagnóstico após falha.",
			Trigger:     "Erro crítico detetado",
			Action:      "Gerar pacote de diagnóstico",
			Target:      "logs do sistema",
			CooldownSec: 120,
			Enabled:     true,
		},
	}
}

// LoadRules lê regras do JSON; cria ficheiro com defaults se não existir.
func LoadRules(path string) ([]Rule, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			out := DefaultRules()
			if saveErr := SaveRules(path, out); saveErr != nil {
				return nil, saveErr
			}
			return out, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(bb))) == 0 {
		return []Rule{}, nil
	}
	var out []Rule
	if err := json.Unmarshal(bb, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveRules grava regras no disco.
func SaveRules(path string, rules []Rule) error {
	bb, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bb, 0o644)
}

// LoadHistory lê linhas de histórico persistidas.
func LoadHistory(path string) ([]string, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(bb))) == 0 {
		return []string{}, nil
	}
	var lines []string
	if err := json.Unmarshal(bb, &lines); err != nil {
		return nil, err
	}
	if lines == nil {
		return []string{}, nil
	}
	return lines, nil
}

// SaveHistory grava histórico completo.
func SaveHistory(path string, lines []string) error {
	if lines == nil {
		lines = []string{}
	}
	bb, err := json.MarshalIndent(lines, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bb, 0o644)
}

// AppendHistory acrescenta evento no topo (máx. 120 linhas).
func AppendHistory(path string, message string) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return nil
	}
	line := time.Now().Format("15:04:05") + "  " + msg
	lines, err := LoadHistory(path)
	if err != nil {
		return err
	}
	lines = append([]string{line}, lines...)
	if len(lines) > 120 {
		lines = lines[:120]
	}
	return SaveHistory(path, lines)
}

// ClearHistory apaga histórico persistido.
func ClearHistory(path string) error {
	return SaveHistory(path, []string{})
}

// Engine executa o loop de verificação de regras.
type Engine struct {
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// Running indica se o motor está ativo.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Start inicia o ticker de automação (idempotente).
func (e *Engine) Start(docker client.APIClient, rulesPath, historyPath string, onEvent func(string)) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	stop := make(chan struct{})
	e.stopCh = stop
	e.mu.Unlock()

	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		lastActionAt := map[string]time.Time{}
		emit := func(msg string) {
			if onEvent != nil {
				onEvent(msg)
			}
			_ = AppendHistory(historyPath, msg)
		}
		emit("Motor de automação iniciado.")
		runOnce(docker, rulesPath, lastActionAt, emit)
		for {
			select {
			case <-stop:
				emit("Motor de automação parado.")
				return
			case <-ticker.C:
				runOnce(docker, rulesPath, lastActionAt, emit)
			}
		}
	}()
}

// Stop encerra o motor.
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	ch := e.stopCh
	e.stopCh = nil
	e.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func runOnce(docker client.APIClient, rulesPath string, lastActionAt map[string]time.Time, onEvent func(string)) {
	if docker == nil {
		onEvent("Docker indisponível nesta conexão.")
		return
	}
	rules, err := LoadRules(rulesPath)
	if err != nil {
		onEvent("Erro ao ler regras: " + err.Error())
		return
	}
	if len(rules) == 0 {
		onEvent("Nenhuma regra configurada.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	containers, err := docker.ContainerList(ctx, dcontainer.ListOptions{All: true})
	if err != nil {
		onEvent("Falha ao listar contêineres: " + err.Error())
		return
	}
	byName := map[string]dcontainer.Summary{}
	for _, c := range containers {
		name := ContainerDisplayName(c)
		if name == "" {
			continue
		}
		byName[strings.ToLower(strings.TrimSpace(name))] = c
	}
	executed := RunTick(ctx, docker, rules, byName, lastActionAt, onEvent)
	if executed == 0 {
		onEvent("Varredura concluída — nenhuma ação necessária.")
	}
}

// RunTick verifica gatilhos e executa ações suportadas; devolve número de ações.
func RunTick(ctx context.Context, docker client.APIClient, rules []Rule, byName map[string]dcontainer.Summary, lastActionAt map[string]time.Time, onEvent func(string)) int {
	executed := 0
	for _, rule := range rules {
		if !rule.Enabled || rule.Kind != KindDockerStoppedRestart {
			continue
		}
		target := strings.ToLower(strings.TrimSpace(rule.Target))
		if target == "" {
			continue
		}
		c, ok := byName[target]
		if !ok {
			continue
		}
		state := strings.ToLower(strings.TrimSpace(string(c.State)))
		if state == "running" || state == "restarting" {
			continue
		}
		key := rule.ID + "::" + c.ID
		cooldown := time.Duration(rule.CooldownSec) * time.Second
		if cooldown < 10*time.Second {
			cooldown = 10 * time.Second
		}
		last := lastActionAt[key]
		if !last.IsZero() && time.Since(last) < cooldown {
			continue
		}
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			rctx, rcancel := context.WithTimeout(context.Background(), 40*time.Second)
			err = docker.ContainerRestart(rctx, c.ID, dcontainer.StopOptions{})
			rcancel()
			if err == nil {
				break
			}
			if attempt < 3 {
				backoff := time.Duration(attempt) * 2 * time.Second
				onEvent(fmt.Sprintf("Tentativa %d para %s — nova tentativa em %s", attempt, target, backoff))
				time.Sleep(backoff)
			}
		}
		if err != nil {
			onEvent(fmt.Sprintf("Falha ao reiniciar %s: %v", target, err))
			continue
		}
		lastActionAt[key] = time.Now()
		executed++
		msg := fmt.Sprintf("Reiniciado %s (regra: %s)", target, rule.Name)
		onEvent(msg)
		if hook := strings.TrimSpace(rule.WebhookURL); hook != "" {
			msgCopy := msg
			ruleName := rule.Name
			go func() {
				hctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				if werr := PostWebhook(hctx, hook, ruleName, target, msgCopy); werr != nil {
					onEvent(fmt.Sprintf("Webhook falhou (%s): %v", ruleName, werr))
				}
			}()
		}
	}
	return executed
}

// PostWebhook envia notificação HTTP JSON após ação bem-sucedida.
func PostWebhook(ctx context.Context, urlStr, ruleName, target, message string) error {
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return nil
	}
	payload := map[string]string{
		"source":  "containerway",
		"rule":    ruleName,
		"target":  target,
		"message": message,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ContainerDisplayName devolve nome curto do contêiner (Swarm/Compose ou nome Docker).
func ContainerDisplayName(c dcontainer.Summary) string {
	if c.Labels != nil {
		if v := strings.TrimSpace(c.Labels["com.docker.swarm.service.name"]); v != "" {
			return v
		}
		if v := strings.TrimSpace(c.Labels["com.docker.compose.service"]); v != "" {
			return v
		}
	}
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimSpace(strings.TrimPrefix(c.Names[0], "/"))
	}
	if name == "" {
		return ""
	}
	if len(name) > 48 {
		if i := strings.IndexByte(name, '.'); i > 0 {
			return name[:i]
		}
	}
	return name
}
