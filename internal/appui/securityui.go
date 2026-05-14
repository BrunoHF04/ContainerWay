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
		"Primeiros passos",
		"Bem-vindo ao ContainerWay.\n\n"+
			"• Altere a senha do utilizador admin em Usuários (sessão de admin).\n"+
			"• Configure alertas por e-mail se precisar de registo de acessos.\n"+
			"• Em ligações SSH use known_hosts em ambientes sensíveis.\n"+
			"• Política opcional: ficheiro policy.json na pasta ContainerWay das preferências, ou variável CONTAINERWAY_FORBID_INSECURE_HOSTKEY=1.\n\n"+
			"Consulte também o manual (? no explorador).",
		w,
	)
	app.Preferences().SetBool(firstRunTipsPreferenceKey, true)
}

// showSecurityPolicyDialog resume políticas locais e caminhos de configuração.
func (ui *explorer) showSecurityPolicyDialog() {
	cfgDir, err := os.UserConfigDir()
	var b strings.Builder
	b.WriteString("Política local (não depende do servidor SSH):\n\n")
	if err != nil {
		b.WriteString("Não foi possível localizar a pasta de configuração.\n\n")
	} else {
		p := filepath.Join(cfgDir, "ContainerWay", "policy.json")
		b.WriteString("Ficheiro JSON opcional:\n")
		b.WriteString(p)
		b.WriteString("\nExemplo: {\"forbidInsecureHostKey\":true}\n\n")
	}
	b.WriteString("Variável de ambiente:\nCONTAINERWAY_FORBID_INSECURE_HOSTKEY=1\n\n")
	if policy.ForbidInsecureHostKey() {
		b.WriteString("Estado atual: ignorar chave de host está bloqueado.")
	} else {
		b.WriteString("Estado atual: sem bloqueio explícito da opção insegura na ligação.")
	}
	dialog.ShowInformation("Política e segurança", b.String(), ui.win)
}

// showCompareFoldersExplorer abre relatório de diferenças entre as pastas dos dois painéis.
func (ui *explorer) showCompareFoldersExplorer() {
	leftTitle := "Computador local"
	rightTitle := "Servidor"
	if !ui.hostMode {
		rightTitle = "Contêiner"
	}
	text := buildFolderCompareReport(leftTitle, rightTitle, ui.leftPath, ui.rightPath, ui.leftRows, ui.rightRows)
	lbl := widget.NewLabel(text)
	lbl.Wrapping = fyne.TextWrapWord
	scroll := fynecontainer.NewScroll(lbl)
	scroll.SetMinSize(fyne.NewSize(720, 420))
	dialog.NewCustom("Comparar pastas atuais", "Fechar", scroll, ui.win).Show()
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
