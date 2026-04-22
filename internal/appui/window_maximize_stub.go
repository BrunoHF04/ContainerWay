//go:build !windows && !darwin

package appui

import "fyne.io/fyne/v2"

func maximizeMainWindow(w fyne.Window) {
	_ = w
}

func restoreNormalMainWindow(w fyne.Window) {
	_ = w
}
