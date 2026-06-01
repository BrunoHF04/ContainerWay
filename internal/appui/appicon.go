package appui

import (
	_ "embed"
	"sync"

	"fyne.io/fyne/v2"
)

//go:embed window-icon.png
var windowIconPNG []byte

var (
	iconOnce sync.Once
	iconRes  fyne.Resource
)

// appWindowIcon devolve o ícone da mascote para a janela e a aplicação.
func appWindowIcon() fyne.Resource {
	iconOnce.Do(func() {
		if len(windowIconPNG) > 0 {
			iconRes = fyne.NewStaticResource("containerway.png", windowIconPNG)
		}
	})
	return iconRes
}
