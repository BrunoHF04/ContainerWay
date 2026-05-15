package appui

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"containerway/internal/policy"
)

// maybeShowFirstRunTips mostra dicas uma vez após o primeiro login de acesso.
func maybeShowFirstRunTips(w fyne.Window) {
	app := fyne.CurrentApp()
	if app.Preferences().BoolWithFallback(firstRunTipsPreferenceKey, false) {
		return
	}
	dialog.ShowInformation(
		tr("sec_first_run_title"),
		tr("sec_first_run_body"),
		w,
	)
	app.Preferences().SetBool(firstRunTipsPreferenceKey, true)
}

// showSecurityPolicyDialog resume políticas locais e caminhos de configuração.
func (ui *explorer) showSecurityPolicyDialog() {
	cfgDir, err := os.UserConfigDir()
	var b strings.Builder
	b.WriteString(tr("sec_policy_head"))
	if err != nil {
		b.WriteString(tr("sec_policy_cfg_err"))
	} else {
		p := filepath.Join(cfgDir, "ContainerWay", "policy.json")
		b.WriteString(tr("sec_policy_file_intro"))
		b.WriteString(p)
		b.WriteString(tr("sec_policy_file_example"))
	}
	b.WriteString(tr("sec_policy_env"))
	if policy.ForbidInsecureHostKey() {
		b.WriteString(tr("sec_policy_state_block"))
	} else {
		b.WriteString(tr("sec_policy_state_allow"))
	}
	dialog.ShowInformation(tr("sec_policy_dlg_title"), b.String(), ui.win)
}

// showCompareFoldersExplorer abre relatório de diferenças entre as pastas dos dois painéis.
func (ui *explorer) showCompareFoldersExplorer() {
	leftTitle := tr("ex_pane_local")
	rightTitle := tr("ex_compare_server")
	if !ui.hostMode {
		rightTitle = tr("ex_compare_container")
	}
	text := buildFolderCompareReport(leftTitle, rightTitle, ui.leftPath, ui.rightPath, ui.leftRows, ui.rightRows)
	lbl := widget.NewLabel(text)
	lbl.Wrapping = fyne.TextWrapWord
	scroll := fynecontainer.NewScroll(lbl)
	scroll.SetMinSize(fyne.NewSize(720, 420))
	dialog.NewCustom(tr("compare_dlg_title"), tr("compare_close"), scroll, ui.win).Show()
	appendAuditLog("explorador", fmt.Sprintf("Comparação de pastas: %s vs %s", ui.leftPath, ui.rightPath))
}

// writeAuditLogCSVBytes gera CSV a partir do log de auditoria em texto.
func writeAuditLogCSVBytes(maxLines int, filter string) ([]byte, error) {
	lines := readAuditLogLines(maxLines, filter)
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"timestamp", "level", "scope", "operator", "message"}); err != nil {
		return nil, err
	}
	for _, line := range lines {
		parts := strings.SplitN(line, " | ", 5)
		if len(parts) < 5 {
			if err := w.Write([]string{line, "", "", "", ""}); err != nil {
				return nil, err
			}
			continue
		}
		op := strings.TrimPrefix(parts[3], "operador=")
		if err := w.Write([]string{parts[0], parts[1], parts[2], op, parts[4]}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}
