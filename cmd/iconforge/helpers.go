package main

import (
	"image"
	"image/color"
	"math"
)

// drawArrow desenha uma seta para baixo.
func drawArrow(img *image.NRGBA, cx, y, width float64, c color.NRGBA) {
	h := width * 0.32
	half := width / 2
	for yy := y - h/2; yy < y+h/2; yy++ {
		for xx := cx - half*0.42; xx < cx+half*0.42; xx++ {
			setPixelBlend(img, int(xx), int(yy), c)
		}
	}
	for i := 0.0; i < half; i++ {
		x1 := cx - i*0.62
		x2 := cx + i*0.62
		yy := y + h/2 + i*0.55
		for xx := x1; xx <= x2; xx++ {
			setPixelBlend(img, int(xx), int(yy), c)
		}
	}
}

// drawRoundedRect preenche um retângulo com cantos arredondados.
func drawRoundedRect(img *image.NRGBA, x, y, w, h, r float64, c color.NRGBA) {
	for yy := int(y); yy < int(y+h); yy++ {
		for xx := int(x); xx < int(x+w); xx++ {
			if insideRoundedRect(float64(xx)+0.5, float64(yy)+0.5, x, y, w, h, r) {
				setPixelBlend(img, xx, yy, c)
			}
		}
	}
}

// insideRoundedRect testa se o ponto está dentro do retângulo arredondado.
func insideRoundedRect(px, py, x, y, w, h, r float64) bool {
	if px < x || px > x+w || py < y || py > y+h {
		return false
	}
	if r <= 0 {
		return true
	}
	left, right := x+r, x+w-r
	top, bottom := y+r, y+h-r
	if (px >= left && px <= right) || (py >= top && py <= bottom) {
		return true
	}
	corners := [4][2]float64{
		{left, top},
		{right, top},
		{left, bottom},
		{right, bottom},
	}
	for _, c := range corners {
		dx := px - c[0]
		dy := py - c[1]
		if dx*dx+dy*dy <= r*r {
			return true
		}
	}
	return false
}

// setPixelBlend mistura um pixel com alpha.
func setPixelBlend(img *image.NRGBA, x, y int, src color.NRGBA) {
	if !(image.Pt(x, y).In(img.Rect)) {
		return
	}
	dst := img.NRGBAAt(x, y)
	a := float64(src.A) / 255.0
	na := 1 - a
	out := color.NRGBA{
		R: uint8(float64(src.R)*a + float64(dst.R)*na),
		G: uint8(float64(src.G)*a + float64(dst.G)*na),
		B: uint8(float64(src.B)*a + float64(dst.B)*na),
		A: uint8(math.Min(255, float64(src.A)+float64(dst.A)*na)),
	}
	img.SetNRGBA(x, y, out)
}

// lerp interpola duas cores NRGBA.
func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	t = clamp01(t)
	return color.NRGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

// clamp01 limita valor entre 0 e 1.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

type bytesBuffer struct {
	buf []byte
}

// Write acrescenta bytes ao buffer.
func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// Bytes devolve o conteúdo do buffer.
func (b *bytesBuffer) Bytes() []byte {
	return b.buf
}
