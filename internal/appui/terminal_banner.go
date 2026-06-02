package appui

import (
	"containerway/internal/termbanner"

	"fyne.io/fyne/v2"
)

// shellStartWithTerminalBanner devolve o comando remoto: cd, MOTD com ASCII + stats ANSI
// e bash de login interativo.
func shellStartWithTerminalBanner(currentDir, sshHost string) string {
	lang := langPTBR
	if a := fyne.CurrentApp(); a != nil {
		lang = loadUILanguage(a)
	}
	return termbanner.ShellStart(currentDir, sshHost, lang)
}
