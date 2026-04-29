//go:build !windows && !darwin

package appui

import "fyne.io/fyne/v2"

// maximizeMainWindow executa parte da logica deste modulo.
func maximizeMainWindow(w fyne.Window) {
	_ = w
}

// restoreNormalMainWindow executa parte da logica deste modulo.
func restoreNormalMainWindow(w fyne.Window) {
	_ = w
}
