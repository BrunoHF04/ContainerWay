package main

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
)

const (
	mascotSourceLight = "assets/mascot-source-light.png"
	mascotSourceDark  = "assets/mascot-source-dark.png"
	mascotHeroSize    = 512
)

var mascotCache = map[string]image.Image{}

// loadMascotSource carrega o PNG mestre da mascote (com cache em memória).
func loadMascotSource(path string) image.Image {
	if img, ok := mascotCache[path]; ok {
		return img
	}
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}
	mascotCache[path] = img
	return img
}

func loadMascotDark() image.Image {
	return loadMascotSource(filepath.Clean(mascotSourceDark))
}

func loadMascotLight() image.Image {
	return loadMascotSource(filepath.Clean(mascotSourceLight))
}

// buildIcon gera o ícone desktop (fundo escuro, mascote recortada).
func buildIcon(size int) image.Image {
	return fitMascotSolid(loadMascotDark(), size, false, bgDark())
}

// buildWebIcon gera ícone web tema escuro.
func buildWebIcon(size int) image.Image {
	return fitMascotSolid(loadMascotDark(), size, false, bgDark())
}

// buildWebIconLight gera ícone web tema claro.
func buildWebIconLight(size int) image.Image {
	return fitMascotSolid(loadMascotLight(), size, true, bgLight())
}

// buildMascotHero gera mascote só com alpha (login / UI).
func buildMascotHero(size int, light bool) image.Image {
	if light {
		return fitMascotTransparent(loadMascotLight(), size, true)
	}
	return fitMascotTransparent(loadMascotDark(), size, false)
}

// fitMascot mantém API usada internamente; transparente por omissão.
func fitMascot(src image.Image, size int, lightSource bool) image.Image {
	return fitMascotTransparent(src, size, lightSource)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// scaleToRect redimensiona com suavização (tamanhos muito pequenos).
func scaleToRect(src image.Image, w, h int) image.Image {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	b := src.Bounds()
	if b.Dx() == w && b.Dy() == h {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := b.Min.Y + y*b.Dy()/h
		for x := 0; x < w; x++ {
			sx := b.Min.X + x*b.Dx()/w
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// scaleIcon redimensiona quadrado (vizinho mais próximo acima de 64px para nitidez).
func scaleIcon(src image.Image, size int) image.Image {
	if size <= 0 {
		return src
	}
	if size >= 64 {
		return scaleIconNearest(src, size)
	}
	b := src.Bounds()
	if b.Dx() == size && b.Dy() == size {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		sy := b.Min.Y + y*b.Dy()/size
		for x := 0; x < size; x++ {
			sx := b.Min.X + x*b.Dx()/size
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
