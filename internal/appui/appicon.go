package appui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"fyne.io/fyne/v2"
)

var (
	iconOnce sync.Once
	iconRes  fyne.Resource
)

// appWindowIcon devolve um PNG 64×64 (fundo escuro + barras ciano) para a janela e a app.
func appWindowIcon() fyne.Resource {
	iconOnce.Do(func() {
		var buf bytes.Buffer
		if err := png.Encode(&buf, buildAppIconImage()); err == nil {
			iconRes = fyne.NewStaticResource("containerway.png", buf.Bytes())
		}
	})
	return iconRes
}

// buildAppIconImage executa parte da logica deste modulo.
func buildAppIconImage() image.Image {
	const s = 64
	img := image.NewNRGBA(image.Rect(0, 0, s, s))
	bg := color.NRGBA{R: 15, G: 23, B: 42, A: 255}
	accent := color.NRGBA{R: 56, G: 189, B: 248, A: 255}
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			img.Set(x, y, bg)
		}
	}
	// “Stack” de três barras (contêineres / camadas)
	for i := 0; i < 3; i++ {
		y0 := 14 + i*14
		for y := y0; y < y0+9; y++ {
			for x := 10; x < 54; x++ {
				img.Set(x, y, accent)
			}
		}
	}
	return img
}
