//go:build windows

package appui

import (
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

const (
	swMaximize = 3
	swRestore  = 9 // restaura de maximizado/minimizado para tamanho normal
)

// restoreNormalMainWindow tira a janela do estado maximizado antes de Resize/Center (ex.: tela Início da sessão).
func restoreNormalMainWindow(w fyne.Window) {
	if w == nil {
		return
	}
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		c, ok := ctx.(driver.WindowsWindowContext)
		if !ok || c.HWND == 0 {
			return
		}
		user32 := syscall.NewLazyDLL("user32.dll")
		showWindow := user32.NewProc("ShowWindow")
		_, _, _ = showWindow.Call(c.HWND, swRestore)
	})
}

func maximizeMainWindow(w fyne.Window) {
	if w == nil {
		return
	}
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		c, ok := ctx.(driver.WindowsWindowContext)
		if !ok || c.HWND == 0 {
			return
		}
		user32 := syscall.NewLazyDLL("user32.dll")
		showWindow := user32.NewProc("ShowWindow")
		_, _, _ = showWindow.Call(c.HWND, swMaximize)
	})
}
