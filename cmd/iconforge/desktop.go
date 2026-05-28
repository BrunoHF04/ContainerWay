package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// buildIcon gera o ícone da aplicação desktop Fyne.
func buildIcon(size int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	baseA := color.NRGBA{R: 15, G: 23, B: 42, A: 255}
	baseB := color.NRGBA{R: 30, G: 41, B: 59, A: 255}
	glow := color.NRGBA{R: 34, G: 211, B: 238, A: 255}
	glow2 := color.NRGBA{R: 56, G: 189, B: 248, A: 255}

	for y := 0; y < size; y++ {
		t := float64(y) / float64(size-1)
		c := lerp(baseA, baseB, t)
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, c)
		}
	}

	radius := float64(size) * 0.12
	cx, cy := float64(size)/2, float64(size)/2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			if !insideRoundedRect(px, py, 0, 0, float64(size), float64(size), radius) {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
				continue
			}
			dx := px - cx
			dy := py - cy
			dist := math.Sqrt(dx*dx+dy*dy) / (float64(size) * 0.75)
			a := clamp01(1.0 - dist)
			cur := img.NRGBAAt(x, y)
			mix := lerp(cur, glow, a*0.14)
			img.SetNRGBA(x, y, mix)
		}
	}

	cardW := float64(size) * 0.62
	cardH := float64(size) * 0.19
	left := (float64(size) - cardW) / 2
	top1 := float64(size)*0.28 - cardH*0.5
	top2 := float64(size)*0.52 - cardH*0.5
	rr := float64(size) * 0.045

	drawRoundedRect(img, left, top1, cardW, cardH, rr, color.NRGBA{R: 8, G: 145, B: 178, A: 255})
	drawRoundedRect(img, left, top2, cardW, cardH, rr, color.NRGBA{R: 14, G: 165, B: 233, A: 255})

	drawRoundedRect(img, left+cardW*0.08, top1+cardH*0.3, cardW*0.25, cardH*0.17, rr*0.55, glow2)
	drawRoundedRect(img, left+cardW*0.08, top2+cardH*0.3, cardW*0.25, cardH*0.17, rr*0.55, glow)

	arrow := image.NewNRGBA(img.Bounds())
	drawArrow(arrow, float64(size)*0.5, float64(size)*0.66, float64(size)*0.26, color.NRGBA{R: 125, G: 211, B: 252, A: 245})
	draw.Draw(img, img.Bounds(), arrow, image.Point{}, draw.Over)

	return img
}
