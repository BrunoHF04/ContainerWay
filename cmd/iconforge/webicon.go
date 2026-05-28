package main

import (
	"image"
	"image/color"
	"math"
)

// buildWebIcon gera ícone da versão browser (painéis dual + destaque web).
func buildWebIcon(size int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)

	bgTop := color.NRGBA{R: 15, G: 20, B: 25, A: 255}
	bgBot := color.NRGBA{R: 26, G: 35, B: 50, A: 255}
	accent := color.NRGBA{R: 59, G: 130, B: 246, A: 255}
	accentHi := color.NRGBA{R: 96, G: 165, B: 250, A: 255}
	panel := color.NRGBA{R: 36, G: 48, B: 68, A: 255}
	panelLine := color.NRGBA{R: 147, G: 197, B: 253, A: 220}
	white := color.NRGBA{R: 248, G: 250, B: 252, A: 255}

	for y := 0; y < size; y++ {
		t := float64(y) / (s - 1)
		c := lerp(bgTop, bgBot, t)
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, c)
		}
	}

	radius := s * 0.2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			if !insideRoundedRect(px, py, 0, 0, s, s, radius) {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
			}
		}
	}

	// Halo suave
	cx, cy := s/2, s/2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			d := math.Hypot(px-cx, py-cy) / (s * 0.72)
			glow := clamp01(1 - d)
			cur := img.NRGBAAt(x, y)
			img.SetNRGBA(x, y, lerp(cur, accent, glow*0.12))
		}
	}

	pad := s * 0.14
	gap := s * 0.06
	panelW := (s - 2*pad - gap) / 2
	panelH := s * 0.52
	panelY := s*0.26 - panelH/2
	pr := s * 0.055

	drawRoundedRect(img, pad, panelY, panelW, panelH, pr, panel)
	drawRoundedRect(img, pad+panelW+gap, panelY, panelW, panelH, pr, panel)

	lineH := panelH * 0.11
	lineGap := panelH * 0.16
	for i := 0; i < 3; i++ {
		ly := panelY + panelH*0.22 + float64(i)*(lineH+lineGap)
		lw := panelW * (0.55 + float64(i)*0.12)
		drawRoundedRect(img, pad+panelW*0.12, ly, lw, lineH, lineH*0.35, panelLine)
		lw2 := panelW * (0.5 + float64(2-i)*0.1)
		drawRoundedRect(img, pad+panelW+gap+panelW*0.12, ly, lw2, lineH, lineH*0.35, panelLine)
	}

	// Divisor central
	divW := s * 0.018
	drawRoundedRect(img, cx-divW/2, panelY+panelH*0.15, divW, panelH*0.7, divW, color.NRGBA{R: 71, G: 85, B: 105, A: 255})

	// Setas ↔
	drawBidirectionalArrow(img, cx, s*0.78, s*0.22, accentHi)

	// Mini “janela browser” no canto
	bw := s * 0.28
	bh := s * 0.2
	bx := s - pad - bw
	by := pad
	drawRoundedRect(img, bx, by, bw, bh, s*0.04, color.NRGBA{R: 30, G: 41, B: 59, A: 240})
	drawRoundedRect(img, bx, by, bw, bh*0.28, s*0.04, accent)
	dotR := s * 0.018
	for i, c := range []color.NRGBA{
		{R: 248, G: 113, B: 113, A: 255},
		{R: 250, G: 204, B: 21, A: 255},
		{R: 74, G: 222, B: 128, A: 255},
	} {
		dx := bx + bw*0.18 + float64(i)*bw*0.14
		dy := by + bh*0.14
		fillCircle(img, dx, dy, dotR, c)
	}
	drawRoundedRect(img, bx+bw*0.1, by+bh*0.42, bw*0.8, bh*0.45, s*0.025, white)

	return img
}

// drawBidirectionalArrow desenha chevrons esquerda e direita.
func drawBidirectionalArrow(img *image.NRGBA, cx, y, width float64, c color.NRGBA) {
	half := width * 0.38
	thick := width * 0.09
	drawChevron(img, cx-half*0.55, y, half*0.5, thick, true, c)
	drawChevron(img, cx+half*0.55, y, half*0.5, thick, false, c)
}

// drawChevron desenha um chevron apontando para a esquerda ou direita.
func drawChevron(img *image.NRGBA, cx, y, arm, thick float64, left bool, c color.NRGBA) {
	dir := 1.0
	if left {
		dir = -1
	}
	for t := 0.0; t < arm; t++ {
		yy1 := y - t*0.55
		yy2 := y + t*0.55
		xx := cx + dir*t
		for d := -thick / 2; d <= thick/2; d++ {
			setPixelBlend(img, int(xx), int(yy1+d), c)
			setPixelBlend(img, int(xx), int(yy2+d), c)
		}
	}
}

// fillCircle preenche um círculo sólido.
func fillCircle(img *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	r2 := r * r
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			if dx*dx+dy*dy <= r2 {
				setPixelBlend(img, x, y, c)
			}
		}
	}
}

// scaleIcon redimensiona o ícone para o tamanho pedido (vizinho mais próximo).
func scaleIcon(src image.Image, size int) image.Image {
	if size <= 0 {
		return src
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
