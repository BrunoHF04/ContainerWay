package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	dcontainer "github.com/docker/docker/api/types/container"
)

const (
	automationKindDockerStoppedRestart = "docker_container_stopped_restart"
)

type automationRule struct {
	ID          string
	Kind        string
	Name        string
	Description string
	Trigger     string
	Action      string
	Target      string
	CooldownSec int
	Enabled     bool
}

// automationConfigPath resolve o arquivo de regras por host.
func (ui *explorer) automationConfigPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível localizar diretório de configuração: %w", err)
	}
	dir := filepath.Join(cfgDir, "ContainerWay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("não foi possível criar diretório de configuração: %w", err)
	}
	host := strings.TrimSpace(ui.connCreds.Host)
	if host == "" {
		host = "host-desconhecido"
	}
	host = strings.ToLower(host)
	host = strings.ReplaceAll(host, ":", "_")
	host = strings.ReplaceAll(host, "/", "_")
	host = strings.ReplaceAll(host, "\\", "_")
	host = strings.ReplaceAll(host, " ", "_")
	return filepath.Join(dir, "automations-"+host+".json"), nil
}

// automationHistoryPath resolve o arquivo de histórico por host.
func (ui *explorer) automationHistoryPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível localizar diretório de configuração: %w", err)
	}
	dir := filepath.Join(cfgDir, "ContainerWay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("não foi possível criar diretório de configuração: %w", err)
	}
	host := strings.TrimSpace(ui.connCreds.Host)
	if host == "" {
		host = "host-desconhecido"
	}
	host = strings.ToLower(host)
	host = strings.ReplaceAll(host, ":", "_")
	host = strings.ReplaceAll(host, "/", "_")
	host = strings.ReplaceAll(host, "\\", "_")
	host = strings.ReplaceAll(host, " ", "_")
	return filepath.Join(dir, "automations-history-"+host+".json"), nil
}

// defaultAutomationRules retorna regras iniciais para o host.
func defaultAutomationRules() []automationRule {
	return []automationRule{
		{
			ID:          "auto-restart-critico",
			Kind:        automationKindDockerStoppedRestart,
			Name:        "Auto-restart de contêiner crítico",
			Description: "Reinicia automaticamente serviço crítico quando o estado sai de execução.",
			Trigger:     "Container parado por mais de 20s",
			Action:      "docker restart",
			Target:      "",
			CooldownSec: 20,
			Enabled:     true,
		},
		{
			ID:          "protecao-disco",
			Kind:        "placeholder_disk_cleanup",
			Name:        "Proteção de disco",
			Description: "Executa limpeza de logs temporários quando uso de disco passa do limite.",
			Trigger:     "Disco /var acima de 85%",
			Action:      "Script de limpeza + notificação",
			Target:      "/var",
			CooldownSec: 60,
			Enabled:     false,
		},
		{
			ID:          "diagnostico-pos-erro",
			Kind:        "placeholder_diagnostic_bundle",
			Name:        "Diagnóstico pós-erro",
			Description: "Coleta evidências quando um serviço principal apresenta falha.",
			Trigger:     "Falha repetida do serviço (3x em 10 min)",
			Action:      "Gerar pacote de diagnóstico + abrir incidente",
			Target:      "serviço principal",
			CooldownSec: 120,
			Enabled:     true,
		},
	}
}

// loadAutomationRules lê as regras persistidas em disco.
func (ui *explorer) loadAutomationRules() ([]automationRule, error) {
	p, err := ui.automationConfigPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			out := defaultAutomationRules()
			if saveErr := ui.saveAutomationRules(out); saveErr != nil {
				return nil, saveErr
			}
			return out, nil
		}
		return nil, fmt.Errorf("não foi possível ler automações: %w", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return []automationRule{}, nil
	}
	var out []automationRule
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("arquivo de automações inválido: %w", err)
	}
	return out, nil
}

// saveAutomationRules grava as regras no arquivo do host.
func (ui *explorer) saveAutomationRules(rules []automationRule) error {
	p, err := ui.automationConfigPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("não foi possível serializar automações: %w", err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return fmt.Errorf("não foi possível salvar automações: %w", err)
	}
	return nil
}

// copyAutomationRules cria cópia independente para leitura concorrente segura.
func copyAutomationRules(in []automationRule) []automationRule {
	out := make([]automationRule, len(in))
	copy(out, in)
	return out
}

// ensureAutomationRules carrega e mantém cache em memória.
func (ui *explorer) ensureAutomationRules() ([]automationRule, error) {
	ui.automationMu.Lock()
	defer ui.automationMu.Unlock()
	if len(ui.automationRules) > 0 {
		return copyAutomationRules(ui.automationRules), nil
	}
	rules, err := ui.loadAutomationRules()
	if err != nil {
		return nil, err
	}
	ui.automationRules = copyAutomationRules(rules)
	return copyAutomationRules(ui.automationRules), nil
}

// replaceAutomationRules substitui regras em memória + disco.
func (ui *explorer) replaceAutomationRules(rules []automationRule) error {
	if err := ui.saveAutomationRules(rules); err != nil {
		return err
	}
	ui.automationMu.Lock()
	ui.automationRules = copyAutomationRules(rules)
	ui.automationMu.Unlock()
	return nil
}

// getAutomationRules retorna snapshot thread-safe das regras em memória.
func (ui *explorer) getAutomationRules() []automationRule {
	ui.automationMu.Lock()
	defer ui.automationMu.Unlock()
	return copyAutomationRules(ui.automationRules)
}

// appendAutomationHistory registra evento local para exibição na UI.
func (ui *explorer) appendAutomationHistory(message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	line := time.Now().Format("15:04:05") + "  " + msg
	ui.automationMu.Lock()
	ui.automationHistory = append([]string{line}, ui.automationHistory...)
	if len(ui.automationHistory) > 120 {
		ui.automationHistory = ui.automationHistory[:120]
	}
	_ = ui.saveAutomationHistoryLocked()
	ui.automationMu.Unlock()
}

// getAutomationHistory retorna cópia dos eventos locais.
func (ui *explorer) getAutomationHistory() []string {
	ui.automationMu.Lock()
	defer ui.automationMu.Unlock()
	out := make([]string, len(ui.automationHistory))
	copy(out, ui.automationHistory)
	return out
}

// clearAutomationHistory limpa eventos locais exibidos na tela.
func (ui *explorer) clearAutomationHistory() {
	ui.automationMu.Lock()
	ui.automationHistory = nil
	_ = ui.saveAutomationHistoryLocked()
	ui.automationMu.Unlock()
}

// loadAutomationHistory carrega histórico persistido do host.
func (ui *explorer) loadAutomationHistory() {
	p, err := ui.automationHistoryPath()
	if err != nil {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	if strings.TrimSpace(string(b)) == "" {
		return
	}
	var rows []string
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	ui.automationMu.Lock()
	ui.automationHistory = rows
	ui.automationMu.Unlock()
}

// saveAutomationHistoryLocked persiste histórico assumindo lock ativo.
func (ui *explorer) saveAutomationHistoryLocked() error {
	p, err := ui.automationHistoryPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(ui.automationHistory, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// automationTargetLabel formata o alvo da regra para exibição.
func automationTargetLabel(r automationRule) string {
	target := strings.TrimSpace(r.Target)
	if target == "" {
		return "não definido"
	}
	return target
}

// startAutomationEngine inicia loop de execução com ticker.
func (ui *explorer) startAutomationEngine(onEvent func(string)) {
	if !ui.automationEngineRunning.CompareAndSwap(false, true) {
		return
	}
	appendAuditLog("automacao", "Motor de automações iniciado")
	stop := make(chan struct{})
	ui.automationEngineMu.Lock()
	ui.automationEngineStop = stop
	ui.automationEngineMu.Unlock()

	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		lastActionAt := map[string]time.Time{}
		onEvent("Motor ativo: monitorando gatilhos a cada 8s.")
		ui.runAutomationTick(lastActionAt, onEvent)
		for {
			select {
			case <-stop:
				onEvent("Motor parado.")
				return
			case <-ticker.C:
				ui.runAutomationTick(lastActionAt, onEvent)
			}
		}
	}()
}

// stopAutomationEngine encerra loop de execução.
func (ui *explorer) stopAutomationEngine() {
	if !ui.automationEngineRunning.CompareAndSwap(true, false) {
		return
	}
	ui.automationEngineMu.Lock()
	ch := ui.automationEngineStop
	ui.automationEngineStop = nil
	ui.automationEngineMu.Unlock()
	if ch != nil {
		close(ch)
	}
	appendAuditLog("automacao", "Motor de automações parado")
}

// runAutomationTick verifica gatilhos e dispara ações suportadas.
func (ui *explorer) runAutomationTick(lastActionAt map[string]time.Time, onEvent func(string)) {
	if ui.s == nil || ui.s.Docker == nil {
		onEvent("Motor: cliente Docker indisponível na sessão atual.")
		return
	}
	rules := ui.getAutomationRules()
	if len(rules) == 0 {
		onEvent("Motor: sem regras cadastradas.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	containers, err := ui.s.Docker.ContainerList(ctx, dcontainer.ListOptions{All: true})
	if err != nil {
		onEvent("Motor: falha ao listar contêineres: " + err.Error())
		return
	}

	byName := map[string]dcontainer.Summary{}
	for _, c := range containers {
		name := containerDisplayName(c)
		if name == "" {
			continue
		}
		byName[strings.ToLower(strings.TrimSpace(name))] = c
	}

	executed := 0
	for _, rule := range rules {
		if !rule.Enabled || rule.Kind != automationKindDockerStoppedRestart {
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
			err = ui.s.Docker.ContainerRestart(rctx, c.ID, dcontainer.StopOptions{})
			rcancel()
			if err == nil {
				break
			}
			if attempt < 3 {
				backoff := time.Duration(attempt) * 2 * time.Second
				onEvent(fmt.Sprintf("Motor: tentativa %d/3 falhou para '%s', retry em %s.", attempt, target, backoff))
				time.Sleep(backoff)
			}
		}
		if err != nil {
			onEvent(fmt.Sprintf("Motor: falha ao reiniciar '%s': %v", target, err))
			continue
		}
		lastActionAt[key] = time.Now()
		executed++
		msg := fmt.Sprintf("Ação automática: '%s' reiniciado pela regra '%s'.", target, rule.Name)
		onEvent(msg)
		appendAuditLog("automacao", msg)
	}
	if executed == 0 {
		onEvent("Motor: varredura concluída, sem ações necessárias.")
	}
}

// showAutomationCenter abre a tela da central de automações.
func (ui *explorer) showAutomationCenter() {
	appendAuditLog("automacao", "Central de automações aberta")

	host := strings.TrimSpace(ui.connCreds.Host)
	if host == "" {
		host = "(host não informado)"
	}
	rules, err := ui.ensureAutomationRules()
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	compact := ui.useCompactLayout()
	ui.loadAutomationHistory()
	if len(ui.getAutomationHistory()) == 0 {
		ui.appendAutomationHistory("Central de automações aberta.")
	}

	hint := widget.NewLabel(
		"Crie regras com gatilho e ação para reduzir tarefas manuais. MVP atual já salva regras e executa auto-restart de contêiner parado.",
	)
	hint.Wrapping = fyne.TextWrapWord

	totalLbl := widget.NewLabel("")
	totalLbl.Wrapping = fyne.TextWrapOff
	statsLbl := widget.NewLabel("")
	statsLbl.Wrapping = fyne.TextWrapOff
	engineLbl := widget.NewLabel("Motor: parado")
	engineLbl.Wrapping = fyne.TextWrapOff
	engineLbl.TextStyle = fyne.TextStyle{Bold: true}
	historyTitle := widget.NewLabelWithStyle("Histórico recente", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	historyRows := ui.getAutomationHistory()
	historyList := widget.NewList(
		func() int { return len(historyRows) },
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("evento")
			lbl.Wrapping = fyne.TextWrapWord
			return lbl
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			lbl := obj.(*widget.Label)
			if id < 0 || int(id) >= len(historyRows) {
				lbl.SetText("")
				return
			}
			lbl.SetText(historyRows[id])
		},
	)
	refreshHistory := func() {
		historyRows = ui.getAutomationHistory()
		historyList.Refresh()
		for i := 0; i < len(historyRows); i++ {
			historyList.SetItemHeight(widget.ListItemID(i), 30)
		}
	}
	refreshHistory()
	btnClearHistory := widget.NewButtonWithIcon("Limpar histórico", theme.DeleteIcon(), func() {
		dialog.ShowConfirm(
			"Limpar histórico",
			"Deseja remover os eventos exibidos no histórico?",
			func(ok bool) {
				if !ok {
					return
				}
				ui.clearAutomationHistory()
				ui.appendAutomationHistory("Histórico limpo manualmente.")
				refreshHistory()
			},
			ui.win,
		)
	})
	btnClearHistory.Importance = widget.WarningImportance
	btnExportHistory := widget.NewButtonWithIcon("Exportar histórico", theme.DownloadIcon(), func() {
		saveDlg := dialog.NewFileSave(func(dst fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(fmt.Errorf("falha ao preparar exportação: %w", err), ui.win)
				return
			}
			if dst == nil {
				return
			}
			defer dst.Close()
			rows := ui.getAutomationHistory()
			if len(rows) == 0 {
				rows = []string{"(histórico vazio)"}
			}
			text := strings.Join(rows, "\n") + "\n"
			if _, wErr := io.WriteString(dst, text); wErr != nil {
				dialog.ShowError(fmt.Errorf("falha ao salvar histórico: %w", wErr), ui.win)
				return
			}
			dialog.ShowInformation("Histórico", "Histórico exportado com sucesso.", ui.win)
		}, ui.win)
		saveDlg.SetFileName("automations-history.txt")
		saveDlg.Show()
	})
	btnExportHistory.Importance = widget.MediumImportance
	statusFilter := widget.NewSelect([]string{"Todas", "Ativas", "Desativadas"}, nil)
	statusFilter.SetSelected("Todas")
	search := widget.NewEntry()
	search.SetPlaceHolder("Pesquisar automações (nome, gatilho, ação)…")
	btnHelp := widget.NewButtonWithIcon("", theme.HelpIcon(), func() {
		dialog.ShowInformation(
			"Sobre a Central de automações",
			"Esta tela serve para criar regras operacionais com gatilho + ação.\n\n"+
				"Já disponível:\n"+
				"- Visualização, pesquisa e criação rápida de automações\n"+
				"- Persistência por host em arquivo JSON\n"+
				"- Motor MVP para auto-restart de contêiner parado\n\n"+
				"Pendente (próximas evoluções):\n"+
				"- Mais tipos de gatilho e ação (disco, logs, alertas)\n"+
				"- Histórico completo de execuções\n"+
				"- Edição/remoção avançada das regras",
			ui.win,
		)
	})
	btnHelp.Importance = widget.LowImportance

	filtered := append([]automationRule(nil), rules...)
	selectedRuleID := ""
	list := widget.NewList(
		func() int { return len(filtered) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("automação")
			title.TextStyle = fyne.TextStyle{Bold: true}
			title.Wrapping = fyne.TextWrapWord
			desc := widget.NewLabel("descrição")
			desc.Wrapping = fyne.TextWrapWord
			meta := widget.NewLabel("gatilho/ação")
			meta.Wrapping = fyne.TextWrapWord
			body := container.NewVBox(
				title,
				widget.NewSeparator(),
				desc,
				meta,
			)
			return panelCard(container.NewPadded(body))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || int(id) >= len(filtered) {
				return
			}
			row := filtered[id]
			card := obj.(*fyne.Container)
			padded := card.Objects[1].(*fyne.Container)
			box := padded.Objects[0].(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			desc := box.Objects[2].(*widget.Label)
			meta := box.Objects[3].(*widget.Label)

			status := "Desativada"
			if row.Enabled {
				status = "Ativa"
			}
			title.SetText(fmt.Sprintf("%s  [%s]", row.Name, status))
			desc.SetText(row.Description)
			meta.SetText(fmt.Sprintf("Gatilho: %s\nAção: %s\nAlvo: %s", row.Trigger, row.Action, automationTargetLabel(row)))
		},
	)
	for i := 0; i < len(filtered); i++ {
		list.SetItemHeight(widget.ListItemID(i), 156)
	}
	btnEdit := widget.NewButtonWithIcon("Editar selecionada", theme.DocumentCreateIcon(), nil)
	btnToggleRule := widget.NewButtonWithIcon("Ativar/Desativar", theme.VisibilityIcon(), nil)
	btnDelete := widget.NewButtonWithIcon("Excluir selecionada", theme.DeleteIcon(), nil)
	btnEdit.Importance = widget.MediumImportance
	btnToggleRule.Importance = widget.MediumImportance
	btnDelete.Importance = widget.DangerImportance
	btnEdit.Disable()
	btnToggleRule.Disable()
	btnDelete.Disable()

	updateRuleButtons := func() {
		if strings.TrimSpace(selectedRuleID) == "" {
			btnEdit.Disable()
			btnToggleRule.Disable()
			btnDelete.Disable()
			return
		}
		btnEdit.Enable()
		btnToggleRule.Enable()
		btnDelete.Enable()
	}

	list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || int(id) >= len(filtered) {
			return
		}
		row := filtered[id]
		selectedRuleID = row.ID
		updateRuleButtons()
		dialog.ShowInformation(
			"Detalhes da automação",
			fmt.Sprintf(
				"%s\n\nDescrição: %s\n\nGatilho: %s\nAção: %s\nAlvo: %s\nCooldown: %ds\nTipo: %s",
				row.Name,
				row.Description,
				row.Trigger,
				row.Action,
				automationTargetLabel(row),
				row.CooldownSec,
				row.Kind,
			),
			ui.win,
		)
	}
	list.OnUnselected = func(_ widget.ListItemID) {
		selectedRuleID = ""
		updateRuleButtons()
	}

	applyFilter := func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		mode := strings.TrimSpace(statusFilter.Selected)
		if mode == "" {
			mode = "Todas"
		}
		allRules := ui.getAutomationRules()
		filtered = filtered[:0]
		activeCount := 0
		disabledCount := 0
		for _, p := range allRules {
			if p.Enabled {
				activeCount++
			} else {
				disabledCount++
			}
			if mode == "Ativas" && !p.Enabled {
				continue
			}
			if mode == "Desativadas" && p.Enabled {
				continue
			}
			blob := strings.ToLower(strings.Join([]string{p.Name, p.Description, p.Trigger, p.Action, p.Target, p.Kind}, " "))
			if q == "" || strings.Contains(blob, q) {
				filtered = append(filtered, p)
			}
		}
		totalLbl.SetText(fmt.Sprintf("Mostrando %d automação(ões) no host %s.", len(filtered), host))
		statsLbl.SetText(fmt.Sprintf("Ativas: %d  |  Desativadas: %d", activeCount, disabledCount))
		list.Refresh()
		for i := 0; i < len(filtered); i++ {
			list.SetItemHeight(widget.ListItemID(i), 156)
		}
		selectedRuleID = ""
		list.UnselectAll()
		updateRuleButtons()
	}
	search.OnChanged = applyFilter
	statusFilter.OnChanged = func(_ string) {
		applyFilter(search.Text)
	}
	applyFilter("")

	btnNew := widget.NewButtonWithIcon("Nova automação", theme.ContentAddIcon(), nil)
	btnNew.Importance = widget.HighImportance

	btnToggleEngine := widget.NewButtonWithIcon("Ativar motor", theme.MediaPlayIcon(), nil)
	btnToggleEngine.Importance = widget.HighImportance
	updateEngineUI := func(msg string) {
		if ui.automationEngineRunning.Load() {
			btnToggleEngine.SetText("Parar motor")
			btnToggleEngine.SetIcon(theme.MediaStopIcon())
		} else {
			btnToggleEngine.SetText("Ativar motor")
			btnToggleEngine.SetIcon(theme.MediaPlayIcon())
		}
		if strings.TrimSpace(msg) != "" {
			engineLbl.SetText("Motor: " + msg)
			ui.appendAutomationHistory(msg)
			refreshHistory()
		} else if ui.automationEngineRunning.Load() {
			engineLbl.SetText("Motor: ativo")
		} else {
			engineLbl.SetText("Motor: parado")
		}
	}
	updateEngineUI("")
	btnToggleEngine.OnTapped = func() {
		if ui.automationEngineRunning.Load() {
			ui.stopAutomationEngine()
			updateEngineUI("parado")
			return
		}
		ui.startAutomationEngine(func(msg string) {
			fyne.Do(func() {
				updateEngineUI(msg)
			})
		})
		updateEngineUI("ativando...")
	}

	btnRunbook := widget.NewButtonWithIcon("Executar runbook", theme.MediaPlayIcon(), func() {
		dialog.ShowInformation("Runbook", "Este botão ficará para playbooks guiados em próximas versões.", ui.win)
	})
	btnRunbook.Importance = widget.MediumImportance

	btnPolicies := widget.NewButtonWithIcon("Políticas", theme.WarningIcon(), func() {
		dialog.ShowInformation("Políticas e compliance", "Este botão ficará para validações de políticas em próximas versões.", ui.win)
	})
	btnPolicies.Importance = widget.MediumImportance

	btnNew.OnTapped = func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Ex.: Auto-restart do nginx")
		targetEntry := widget.NewEntry()
		targetEntry.SetPlaceHolder("Nome do contêiner (ex.: nginx)")
		cooldownEntry := widget.NewEntry()
		cooldownEntry.SetPlaceHolder("20")
		cooldownEntry.SetText("20")
		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder("Descrição opcional")
		form := dialog.NewForm(
			"Nova automação",
			"Criar",
			"Cancelar",
			[]*widget.FormItem{
				widget.NewFormItem("Nome", nameEntry),
				widget.NewFormItem("Contêiner alvo", targetEntry),
				widget.NewFormItem("Cooldown (segundos)", cooldownEntry),
				widget.NewFormItem("Descrição", descEntry),
			},
			func(ok bool) {
				if !ok {
					return
				}
				name := strings.TrimSpace(nameEntry.Text)
				target := strings.TrimSpace(targetEntry.Text)
				cooldown, convErr := strconv.Atoi(strings.TrimSpace(cooldownEntry.Text))
				if name == "" || target == "" {
					dialog.ShowInformation("Nova automação", "Preencha nome e contêiner alvo.", ui.win)
					return
				}
				if convErr != nil || cooldown < 10 {
					dialog.ShowInformation("Nova automação", "Cooldown inválido. Use um número inteiro >= 10.", ui.win)
					return
				}
				desc := strings.TrimSpace(descEntry.Text)
				if desc == "" {
					desc = "Regra criada pelo usuário para auto-restart."
				}
				all := ui.getAutomationRules()
				id := fmt.Sprintf("rule-%d", time.Now().UnixNano())
				all = append(all, automationRule{
					ID:          id,
					Kind:        automationKindDockerStoppedRestart,
					Name:        name,
					Description: desc,
					Trigger:     "Container parado por mais de " + strconv.Itoa(cooldown) + "s",
					Action:      "docker restart",
					Target:      target,
					CooldownSec: cooldown,
					Enabled:     true,
				})
				if err := ui.replaceAutomationRules(all); err != nil {
					dialog.ShowError(err, ui.win)
					return
				}
				appendAuditLog("automacao", "Nova regra criada: "+name+" -> "+target)
				ui.appendAutomationHistory("Regra criada: " + name + " -> " + target)
				applyFilter(search.Text)
				refreshHistory()
				dialog.ShowInformation("Nova automação", "Regra criada e salva no host atual.", ui.win)
			},
			ui.win,
		)
		form.Resize(fyne.NewSize(520, 300))
		form.Show()
	}

	findRuleByID := func(id string) (automationRule, bool) {
		for _, r := range ui.getAutomationRules() {
			if r.ID == id {
				return r, true
			}
		}
		return automationRule{}, false
	}

	btnToggleRule.OnTapped = func() {
		if strings.TrimSpace(selectedRuleID) == "" {
			return
		}
		all := ui.getAutomationRules()
		changedName := ""
		nextState := false
		for i := range all {
			if all[i].ID != selectedRuleID {
				continue
			}
			all[i].Enabled = !all[i].Enabled
			changedName = all[i].Name
			nextState = all[i].Enabled
			break
		}
		if changedName == "" {
			dialog.ShowInformation("Automação", "Regra selecionada não foi encontrada.", ui.win)
			return
		}
		if err := ui.replaceAutomationRules(all); err != nil {
			dialog.ShowError(err, ui.win)
			return
		}
		status := "desativada"
		if nextState {
			status = "ativada"
		}
		appendAuditLog("automacao", "Regra "+status+": "+changedName)
		ui.appendAutomationHistory("Regra " + status + ": " + changedName)
		applyFilter(search.Text)
		refreshHistory()
		dialog.ShowInformation("Automação", "Regra "+status+" com sucesso.", ui.win)
	}

	btnDelete.OnTapped = func() {
		if strings.TrimSpace(selectedRuleID) == "" {
			return
		}
		row, ok := findRuleByID(selectedRuleID)
		if !ok {
			dialog.ShowInformation("Automação", "Regra selecionada não foi encontrada.", ui.win)
			return
		}
		dialog.ShowConfirm(
			"Excluir automação",
			"Confirma excluir a regra?\n\n"+row.Name,
			func(confirm bool) {
				if !confirm {
					return
				}
				all := ui.getAutomationRules()
				out := make([]automationRule, 0, len(all))
				for _, r := range all {
					if r.ID != selectedRuleID {
						out = append(out, r)
					}
				}
				if err := ui.replaceAutomationRules(out); err != nil {
					dialog.ShowError(err, ui.win)
					return
				}
				appendAuditLog("automacao", "Regra excluída: "+row.Name)
				ui.appendAutomationHistory("Regra excluída: " + row.Name)
				applyFilter(search.Text)
				refreshHistory()
				dialog.ShowInformation("Automação", "Regra excluída com sucesso.", ui.win)
			},
			ui.win,
		)
	}

	btnEdit.OnTapped = func() {
		if strings.TrimSpace(selectedRuleID) == "" {
			return
		}
		row, ok := findRuleByID(selectedRuleID)
		if !ok {
			dialog.ShowInformation("Automação", "Regra selecionada não foi encontrada.", ui.win)
			return
		}
		nameEntry := widget.NewEntry()
		nameEntry.SetText(row.Name)
		targetEntry := widget.NewEntry()
		targetEntry.SetText(row.Target)
		cooldownEntry := widget.NewEntry()
		cooldownEntry.SetText(strconv.Itoa(row.CooldownSec))
		descEntry := widget.NewEntry()
		descEntry.SetText(row.Description)
		form := dialog.NewForm(
			"Editar automação",
			"Salvar",
			"Cancelar",
			[]*widget.FormItem{
				widget.NewFormItem("Nome", nameEntry),
				widget.NewFormItem("Contêiner alvo", targetEntry),
				widget.NewFormItem("Cooldown (segundos)", cooldownEntry),
				widget.NewFormItem("Descrição", descEntry),
			},
			func(ok bool) {
				if !ok {
					return
				}
				name := strings.TrimSpace(nameEntry.Text)
				target := strings.TrimSpace(targetEntry.Text)
				cooldown, convErr := strconv.Atoi(strings.TrimSpace(cooldownEntry.Text))
				desc := strings.TrimSpace(descEntry.Text)
				if name == "" || target == "" {
					dialog.ShowInformation("Editar automação", "Preencha nome e contêiner alvo.", ui.win)
					return
				}
				if convErr != nil || cooldown < 10 {
					dialog.ShowInformation("Editar automação", "Cooldown inválido. Use um número inteiro >= 10.", ui.win)
					return
				}
				if desc == "" {
					desc = "Regra atualizada pelo usuário."
				}
				all := ui.getAutomationRules()
				updated := false
				for i := range all {
					if all[i].ID != selectedRuleID {
						continue
					}
					all[i].Name = name
					all[i].Target = target
					all[i].CooldownSec = cooldown
					all[i].Description = desc
					all[i].Trigger = "Container parado por mais de " + strconv.Itoa(cooldown) + "s"
					updated = true
					break
				}
				if !updated {
					dialog.ShowInformation("Editar automação", "Regra selecionada não foi encontrada.", ui.win)
					return
				}
				if err := ui.replaceAutomationRules(all); err != nil {
					dialog.ShowError(err, ui.win)
					return
				}
				appendAuditLog("automacao", "Regra editada: "+name+" -> "+target)
				ui.appendAutomationHistory("Regra editada: " + name + " -> " + target)
				applyFilter(search.Text)
				refreshHistory()
				dialog.ShowInformation("Editar automação", "Regra atualizada com sucesso.", ui.win)
			},
			ui.win,
		)
		form.Resize(fyne.NewSize(520, 300))
		form.Show()
	}

	filterBar := container.NewBorder(
		nil,
		nil,
		container.NewHBox(widget.NewLabel("Status"), statusFilter),
		btnHelp,
		search,
	)
	if compact {
		filterBar = container.NewBorder(
			nil,
			nil,
			nil,
			btnHelp,
			container.NewVBox(
				search,
				container.NewHBox(widget.NewLabel("Status"), statusFilter, layout.NewSpacer()),
			),
		)
	}

	top := container.NewVBox(
		hint,
		widget.NewSeparator(),
		filterBar,
		container.NewHBox(totalLbl, widget.NewLabel(" | "), statsLbl, layout.NewSpacer()),
		engineLbl,
		widget.NewSeparator(),
	)
	actions := container.NewVBox(
		container.NewHBox(
			btnNew,
			btnToggleEngine,
			btnEdit,
			btnToggleRule,
			btnDelete,
			layout.NewSpacer(),
		),
		container.NewHBox(
			btnRunbook,
			btnPolicies,
			layout.NewSpacer(),
		),
	)
	if !compact {
		actions = container.NewVBox(container.NewHBox(
			btnNew,
			btnToggleEngine,
			btnEdit,
			btnToggleRule,
			btnDelete,
			btnRunbook,
			btnPolicies,
			layout.NewSpacer(),
		))
	}
	mainSplit := container.NewVSplit(
		container.NewVScroll(list),
		container.NewVScroll(historyList),
	)
	if compact {
		mainSplit.SetOffset(0.65)
	} else {
		mainSplit.SetOffset(0.72)
	}

	body := container.NewBorder(
		container.NewPadded(top),
		container.NewPadded(container.NewVBox(widget.NewSeparator(), actions)),
		nil,
		nil,
		container.NewPadded(
			container.NewBorder(
				nil,
				container.NewVBox(
					widget.NewSeparator(),
					container.NewBorder(
						nil,
						nil,
						historyTitle,
						container.NewHBox(btnExportHistory, btnClearHistory, layout.NewSpacer()),
						widget.NewLabel(""),
					),
				),
				nil,
				nil,
				mainSplit,
			),
		),
	)

	ui.openSettingsFullscreenWithBack("Central de automações", body, nil)
}
