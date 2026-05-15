package appui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	WebhookURL  string `json:"webhookURL,omitempty"`
}

// postAutomationWebhook envia POST JSON após uma ação de automação bem-sucedida.
func postAutomationWebhook(ctx context.Context, urlStr, ruleName, target, message string) error {
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

// automationConfigPath resolve o arquivo de regras por host.
func (ui *explorer) automationConfigPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf(tr("mod_auto_err_cfg_lookup"), err)
	}
	dir := filepath.Join(cfgDir, "ContainerWay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf(tr("mod_auto_err_cfg_mkdir"), err)
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
		return "", fmt.Errorf(tr("mod_auto_err_cfg_lookup"), err)
	}
	dir := filepath.Join(cfgDir, "ContainerWay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf(tr("mod_auto_err_cfg_mkdir"), err)
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
			Name:        tr("mod_auto_seed_crit_name"),
			Description: tr("mod_auto_seed_crit_desc"),
			Trigger:     tr("mod_auto_seed_crit_trigger"),
			Action:      "docker restart",
			Target:      "",
			CooldownSec: 20,
			Enabled:     true,
		},
		{
			ID:          "protecao-disco",
			Kind:        "placeholder_disk_cleanup",
			Name:        tr("mod_auto_seed_disk_name"),
			Description: tr("mod_auto_seed_disk_desc"),
			Trigger:     tr("mod_auto_seed_disk_trigger"),
			Action:      tr("mod_auto_seed_disk_action"),
			Target:      "/var",
			CooldownSec: 60,
			Enabled:     false,
		},
		{
			ID:          "diagnostico-pos-erro",
			Kind:        "placeholder_diagnostic_bundle",
			Name:        tr("mod_auto_seed_diag_name"),
			Description: tr("mod_auto_seed_diag_desc"),
			Trigger:     tr("mod_auto_seed_diag_trigger"),
			Action:      tr("mod_auto_seed_diag_action"),
			Target:      tr("mod_auto_seed_diag_target"),
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
		return nil, fmt.Errorf(tr("mod_auto_err_read"), err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return []automationRule{}, nil
	}
	var out []automationRule
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf(tr("mod_auto_err_invalid_json"), err)
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
		return fmt.Errorf(tr("mod_auto_err_serialize"), err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return fmt.Errorf(tr("mod_auto_err_save"), err)
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
		return tr("mod_auto_target_undefined")
	}
	return target
}

// startAutomationEngine inicia loop de execução com ticker.
func (ui *explorer) startAutomationEngine(onEvent func(string)) {
	if !ui.automationEngineRunning.CompareAndSwap(false, true) {
		return
	}
	appendAuditLog("automacao", tr("mod_auto_audit_engine_started"))
	stop := make(chan struct{})
	ui.automationEngineMu.Lock()
	ui.automationEngineStop = stop
	ui.automationEngineMu.Unlock()

	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		lastActionAt := map[string]time.Time{}
		onEvent(tr("mod_auto_engine_tick_active"))
		ui.runAutomationTick(lastActionAt, onEvent)
		for {
			select {
			case <-stop:
				onEvent(tr("mod_auto_engine_tick_stopped"))
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
	appendAuditLog("automacao", tr("mod_auto_audit_engine_stopped"))
}

// runAutomationTick verifica gatilhos e dispara ações suportadas.
func (ui *explorer) runAutomationTick(lastActionAt map[string]time.Time, onEvent func(string)) {
	if ui.s == nil || ui.s.Docker == nil {
		onEvent(tr("mod_auto_engine_docker_unavail"))
		return
	}
	rules := ui.getAutomationRules()
	if len(rules) == 0 {
		onEvent(tr("mod_auto_engine_no_rules"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	containers, err := ui.s.Docker.ContainerList(ctx, dcontainer.ListOptions{All: true})
	if err != nil {
		onEvent(tr("mod_auto_engine_list_fail") + err.Error())
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
				onEvent(fmt.Sprintf(tr("mod_auto_engine_retry_fmt"), attempt, target, backoff))
				time.Sleep(backoff)
			}
		}
		if err != nil {
			onEvent(fmt.Sprintf(tr("mod_auto_engine_restart_fail_fmt"), target, err))
			continue
		}
		lastActionAt[key] = time.Now()
		executed++
		msg := fmt.Sprintf(tr("mod_auto_engine_action_fmt"), target, rule.Name)
		onEvent(msg)
		appendAuditLog("automacao", msg)
		if hook := strings.TrimSpace(rule.WebhookURL); hook != "" {
			msgCopy := msg
			ruleName := rule.Name
			go func() {
				hctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				if werr := postAutomationWebhook(hctx, hook, ruleName, target, msgCopy); werr != nil {
					appendAuditLog("automacao", fmt.Sprintf(tr("mod_auto_engine_webhook_fail_fmt"), ruleName, werr))
				}
			}()
		}
	}
	if executed == 0 {
		onEvent(tr("mod_auto_engine_scan_done"))
	}
}

// showAutomationCenter abre a tela da central de automações.
func (ui *explorer) showAutomationCenter() {
	appendAuditLog("automacao", tr("mod_auto_hist_opened_center"))

	host := strings.TrimSpace(ui.connCreds.Host)
	if host == "" {
		host = tr("mod_auto_host_unnamed")
	}
	rules, err := ui.ensureAutomationRules()
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	compact := ui.useCompactLayout()
	ui.loadAutomationHistory()
	if len(ui.getAutomationHistory()) == 0 {
		ui.appendAutomationHistory(tr("mod_auto_hist_opened_center"))
	}

	hint := widget.NewLabel(
		tr("mod_auto_hint_banner"),
	)
	hint.Wrapping = fyne.TextWrapWord

	totalLbl := widget.NewLabel("")
	totalLbl.Wrapping = fyne.TextWrapOff
	statsLbl := widget.NewLabel("")
	statsLbl.Wrapping = fyne.TextWrapOff
	engineLbl := widget.NewLabel(tr("mod_auto_engine_label_stopped"))
	engineLbl.Wrapping = fyne.TextWrapOff
	engineLbl.TextStyle = fyne.TextStyle{Bold: true}
	historyTitle := widget.NewLabelWithStyle(tr("mod_auto_recent_history"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	historyRows := ui.getAutomationHistory()
	historyList := widget.NewList(
		func() int { return len(historyRows) },
		func() fyne.CanvasObject {
			lbl := widget.NewLabel(tr("mod_auto_hist_row_placeholder"))
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
	btnClearHistory := widget.NewButtonWithIcon(tr("mod_auto_btn_clear_hist"), theme.DeleteIcon(), func() {
		dialog.ShowConfirm(
			tr("mod_auto_hist_clear_title"),
			tr("mod_auto_hist_clear_body"),
			func(ok bool) {
				if !ok {
					return
				}
				ui.clearAutomationHistory()
				ui.appendAutomationHistory(tr("mod_auto_hist_cleared"))
				refreshHistory()
			},
			ui.win,
		)
	})
	btnClearHistory.Importance = widget.WarningImportance
	btnExportHistory := widget.NewButtonWithIcon(tr("mod_auto_btn_export_hist"), theme.DownloadIcon(), func() {
		saveDlg := dialog.NewFileSave(func(dst fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(fmt.Errorf(tr("mod_auto_export_hist_fail"), err), ui.win)
				return
			}
			if dst == nil {
				return
			}
			defer dst.Close()
			rows := ui.getAutomationHistory()
			if len(rows) == 0 {
				rows = []string{tr("mod_auto_hist_empty_line")}
			}
			text := strings.Join(rows, "\n") + "\n"
			if _, wErr := io.WriteString(dst, text); wErr != nil {
				dialog.ShowError(fmt.Errorf(tr("mod_auto_save_hist_fail"), wErr), ui.win)
				return
			}
			dialog.ShowInformation(tr("dlg_history_title"), tr("mod_auto_hist_export_ok"), ui.win)
		}, ui.win)
		saveDlg.SetFileName("automations-history.txt")
		saveDlg.Show()
	})
	btnExportHistory.Importance = widget.MediumImportance
	filterAll := tr("mod_auto_filter_all")
	filterActive := tr("mod_auto_filter_active")
	filterDisabled := tr("mod_auto_filter_disabled")
	statusFilter := widget.NewSelect([]string{filterAll, filterActive, filterDisabled}, nil)
	statusFilter.SetSelected(filterAll)
	search := widget.NewEntry()
	search.SetPlaceHolder(tr("mod_auto_search_ph"))
	btnHelp := widget.NewButtonWithIcon("", theme.HelpIcon(), func() {
		dialog.ShowInformation(
			tr("mod_auto_help_title"),
			tr("mod_auto_help_body"),
			ui.win,
		)
	})
	btnHelp.Importance = widget.LowImportance

	filtered := append([]automationRule(nil), rules...)
	selectedRuleID := ""
	list := widget.NewList(
		func() int { return len(filtered) },
		func() fyne.CanvasObject {
			title := widget.NewLabel(tr("mod_auto_list_row_title"))
			title.TextStyle = fyne.TextStyle{Bold: true}
			title.Wrapping = fyne.TextWrapWord
			desc := widget.NewLabel(tr("mod_auto_list_row_desc"))
			desc.Wrapping = fyne.TextWrapWord
			meta := widget.NewLabel(tr("mod_auto_list_row_meta"))
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

			status := tr("mod_auto_rule_badge_disabled")
			if row.Enabled {
				status = tr("mod_auto_rule_badge_active")
			}
			title.SetText(fmt.Sprintf("%s  [%s]", row.Name, status))
			desc.SetText(row.Description)
			meta.SetText(fmt.Sprintf(tr("mod_auto_meta_lines_fmt"), row.Trigger, row.Action, automationTargetLabel(row)))
		},
	)
	for i := 0; i < len(filtered); i++ {
		list.SetItemHeight(widget.ListItemID(i), 156)
	}
	btnEdit := widget.NewButtonWithIcon(tr("mod_auto_btn_edit_sel"), theme.DocumentCreateIcon(), nil)
	btnToggleRule := widget.NewButtonWithIcon(tr("mod_auto_btn_toggle"), theme.VisibilityIcon(), nil)
	btnDelete := widget.NewButtonWithIcon(tr("mod_auto_btn_delete_sel"), theme.DeleteIcon(), nil)
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
			tr("mod_auto_detail_title"),
			fmt.Sprintf(
				tr("mod_auto_detail_body_fmt"),
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
			mode = filterAll
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
			if mode == filterActive && !p.Enabled {
				continue
			}
			if mode == filterDisabled && p.Enabled {
				continue
			}
			blob := strings.ToLower(strings.Join([]string{p.Name, p.Description, p.Trigger, p.Action, p.Target, p.Kind}, " "))
			if q == "" || strings.Contains(blob, q) {
				filtered = append(filtered, p)
			}
		}
		totalLbl.SetText(fmt.Sprintf(tr("mod_auto_showing_fmt"), len(filtered), host))
		statsLbl.SetText(fmt.Sprintf(tr("mod_auto_stats_fmt"), activeCount, disabledCount))
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

	btnNew := widget.NewButtonWithIcon(tr("mod_auto_btn_new"), theme.ContentAddIcon(), nil)
	btnNew.Importance = widget.HighImportance

	btnToggleEngine := widget.NewButtonWithIcon(tr("mod_auto_btn_engine_start"), theme.MediaPlayIcon(), nil)
	btnToggleEngine.Importance = widget.HighImportance
	updateEngineUI := func(msg string) {
		if ui.automationEngineRunning.Load() {
			btnToggleEngine.SetText(tr("mod_auto_btn_engine_stop"))
			btnToggleEngine.SetIcon(theme.MediaStopIcon())
		} else {
			btnToggleEngine.SetText(tr("mod_auto_btn_engine_start"))
			btnToggleEngine.SetIcon(theme.MediaPlayIcon())
		}
		if strings.TrimSpace(msg) != "" {
			engineLbl.SetText(msg)
			ui.appendAutomationHistory(msg)
			refreshHistory()
		} else if ui.automationEngineRunning.Load() {
			engineLbl.SetText(tr("mod_auto_engine_label_active"))
		} else {
			engineLbl.SetText(tr("mod_auto_engine_label_stopped"))
		}
	}
	updateEngineUI("")
	btnToggleEngine.OnTapped = func() {
		if ui.automationEngineRunning.Load() {
			ui.stopAutomationEngine()
			updateEngineUI("")
			return
		}
		ui.startAutomationEngine(func(msg string) {
			fyne.Do(func() {
				updateEngineUI(msg)
			})
		})
		updateEngineUI(tr("mod_auto_engine_msg_starting"))
	}

	btnRunbook := widget.NewButtonWithIcon(tr("mod_auto_btn_runbook"), theme.MediaPlayIcon(), func() {
		dialog.ShowInformation(tr("mod_auto_title_runbook"), tr("mod_auto_runbook_stub"), ui.win)
	})
	btnRunbook.Importance = widget.MediumImportance

	btnPolicies := widget.NewButtonWithIcon(tr("mod_auto_btn_policies"), theme.WarningIcon(), func() {
		ui.showSecurityPolicyDialog()
	})
	btnPolicies.Importance = widget.MediumImportance

	btnNew.OnTapped = func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder(tr("mod_auto_ph_name_example"))
		targetEntry := widget.NewEntry()
		targetEntry.SetPlaceHolder(tr("mod_auto_ph_target_container"))
		cooldownEntry := widget.NewEntry()
		cooldownEntry.SetPlaceHolder("20")
		cooldownEntry.SetText("20")
		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder(tr("mod_auto_ph_desc_opt"))
		webhookEntry := widget.NewEntry()
		webhookEntry.SetPlaceHolder(tr("mod_auto_ph_webhook"))
		form := dialog.NewForm(
			tr("mod_auto_form_new_title"),
			tr("mod_auto_form_create_btn"),
			tr("mod_auto_form_cancel_btn"),
			[]*widget.FormItem{
				widget.NewFormItem(tr("mod_auto_fi_name"), nameEntry),
				widget.NewFormItem(tr("mod_auto_fi_target"), targetEntry),
				widget.NewFormItem(tr("mod_auto_fi_cooldown"), cooldownEntry),
				widget.NewFormItem(tr("mod_auto_fi_desc"), descEntry),
				widget.NewFormItem(tr("mod_auto_fi_webhook"), webhookEntry),
			},
			func(ok bool) {
				if !ok {
					return
				}
				name := strings.TrimSpace(nameEntry.Text)
				target := strings.TrimSpace(targetEntry.Text)
				cooldown, convErr := strconv.Atoi(strings.TrimSpace(cooldownEntry.Text))
				if name == "" || target == "" {
					dialog.ShowInformation(tr("mod_auto_title_new"), tr("mod_auto_new_fill"), ui.win)
					return
				}
				if convErr != nil || cooldown < 10 {
					dialog.ShowInformation(tr("mod_auto_title_new"), tr("mod_auto_bad_cooldown"), ui.win)
					return
				}
				desc := strings.TrimSpace(descEntry.Text)
				if desc == "" {
					desc = tr("mod_auto_default_desc_new")
				}
				all := ui.getAutomationRules()
				id := fmt.Sprintf("rule-%d", time.Now().UnixNano())
				all = append(all, automationRule{
					ID:          id,
					Kind:        automationKindDockerStoppedRestart,
					Name:        name,
					Description: desc,
					Trigger:     fmt.Sprintf(tr("mod_auto_trigger_stopped_fmt"), cooldown),
					Action:      "docker restart",
					Target:      target,
					CooldownSec: cooldown,
					Enabled:     true,
					WebhookURL:  strings.TrimSpace(webhookEntry.Text),
				})
				if err := ui.replaceAutomationRules(all); err != nil {
					dialog.ShowError(err, ui.win)
					return
				}
				appendAuditLog("automacao", fmt.Sprintf(tr("mod_auto_audit_rule_created_fmt"), name, target))
				ui.appendAutomationHistory(fmt.Sprintf(tr("mod_auto_hist_rule_created_fmt"), name, target))
				applyFilter(search.Text)
				refreshHistory()
				dialog.ShowInformation(tr("mod_auto_title_new"), tr("mod_auto_new_saved"), ui.win)
			},
			ui.win,
		)
		form.Resize(fyne.NewSize(560, 380))
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
			dialog.ShowInformation(tr("mod_auto_title_main"), tr("mod_auto_rule_missing"), ui.win)
			return
		}
		if err := ui.replaceAutomationRules(all); err != nil {
			dialog.ShowError(err, ui.win)
			return
		}
		var histMsg string
		if nextState {
			histMsg = fmt.Sprintf(tr("mod_auto_hist_rule_enabled_fmt"), changedName)
			appendAuditLog("automacao", histMsg)
			ui.appendAutomationHistory(histMsg)
			applyFilter(search.Text)
			refreshHistory()
			dialog.ShowInformation(tr("mod_auto_title_main"), tr("mod_auto_rule_enabled_ok"), ui.win)
			return
		}
		histMsg = fmt.Sprintf(tr("mod_auto_hist_rule_disabled_fmt"), changedName)
		appendAuditLog("automacao", histMsg)
		ui.appendAutomationHistory(histMsg)
		applyFilter(search.Text)
		refreshHistory()
		dialog.ShowInformation(tr("mod_auto_title_main"), tr("mod_auto_rule_disabled_ok"), ui.win)
	}

	btnDelete.OnTapped = func() {
		if strings.TrimSpace(selectedRuleID) == "" {
			return
		}
		row, ok := findRuleByID(selectedRuleID)
		if !ok {
			dialog.ShowInformation(tr("mod_auto_title_main"), tr("mod_auto_rule_missing"), ui.win)
			return
		}
		dialog.ShowConfirm(
			tr("mod_auto_delete_dlg_title"),
			fmt.Sprintf(tr("mod_auto_delete_dlg_body_fmt"), row.Name),
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
				appendAuditLog("automacao", fmt.Sprintf(tr("mod_auto_hist_rule_deleted_fmt"), row.Name))
				ui.appendAutomationHistory(fmt.Sprintf(tr("mod_auto_hist_rule_deleted_fmt"), row.Name))
				applyFilter(search.Text)
				refreshHistory()
				dialog.ShowInformation(tr("mod_auto_delete_title"), tr("mod_auto_deleted"), ui.win)
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
			dialog.ShowInformation(tr("mod_auto_title_main"), tr("mod_auto_rule_missing"), ui.win)
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
		webhookEntry := widget.NewEntry()
		webhookEntry.SetText(row.WebhookURL)
		webhookEntry.SetPlaceHolder(tr("mod_auto_ph_webhook_edit"))
		form := dialog.NewForm(
			tr("mod_auto_form_edit_title"),
			tr("mod_auto_form_save_btn"),
			tr("mod_auto_form_cancel_btn"),
			[]*widget.FormItem{
				widget.NewFormItem(tr("mod_auto_fi_name"), nameEntry),
				widget.NewFormItem(tr("mod_auto_fi_target"), targetEntry),
				widget.NewFormItem(tr("mod_auto_fi_cooldown"), cooldownEntry),
				widget.NewFormItem(tr("mod_auto_fi_desc"), descEntry),
				widget.NewFormItem(tr("mod_auto_fi_webhook"), webhookEntry),
			},
			func(ok bool) {
				if !ok {
					return
				}
				name := strings.TrimSpace(nameEntry.Text)
				target := strings.TrimSpace(targetEntry.Text)
				cooldown, convErr := strconv.Atoi(strings.TrimSpace(cooldownEntry.Text))
				desc := strings.TrimSpace(descEntry.Text)
				hook := strings.TrimSpace(webhookEntry.Text)
				if name == "" || target == "" {
					dialog.ShowInformation(tr("mod_auto_title_edit"), tr("mod_auto_edit_fill"), ui.win)
					return
				}
				if convErr != nil || cooldown < 10 {
					dialog.ShowInformation(tr("mod_auto_title_edit"), tr("mod_auto_edit_bad_cd"), ui.win)
					return
				}
				if desc == "" {
					desc = tr("mod_auto_default_desc_edit")
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
					all[i].WebhookURL = hook
					all[i].Trigger = fmt.Sprintf(tr("mod_auto_trigger_stopped_fmt"), cooldown)
					updated = true
					break
				}
				if !updated {
					dialog.ShowInformation(tr("mod_auto_title_edit"), tr("mod_auto_edit_missing"), ui.win)
					return
				}
				if err := ui.replaceAutomationRules(all); err != nil {
					dialog.ShowError(err, ui.win)
					return
				}
				appendAuditLog("automacao", fmt.Sprintf(tr("mod_auto_hist_rule_edited_fmt"), name, target))
				ui.appendAutomationHistory(fmt.Sprintf(tr("mod_auto_hist_rule_edited_fmt"), name, target))
				applyFilter(search.Text)
				refreshHistory()
				dialog.ShowInformation(tr("mod_auto_title_edit"), tr("mod_auto_edit_saved"), ui.win)
			},
			ui.win,
		)
		form.Resize(fyne.NewSize(560, 380))
		form.Show()
	}

	filterBar := container.NewBorder(
		nil,
		nil,
		container.NewHBox(widget.NewLabel(tr("mod_auto_status_label")), statusFilter),
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
				container.NewHBox(widget.NewLabel(tr("mod_auto_status_label")), statusFilter, layout.NewSpacer()),
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

	ui.openSettingsFullscreenWithBack(tr("mod_auto_screen_title"), body, nil)
}
