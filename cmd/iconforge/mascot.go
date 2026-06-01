package main

import (
	"image"
	"image/color"
	"image/draw"
)

// stripBackground remove o fundo sólido (preto ou branco) e devolve PNG com alpha.
func stripBackground(img image.Image, lightSource bool) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := out.NRGBAAt(x, y)
			lum := (float64(c.R) + float64(c.G) + float64(c.B)) / 3

			var alpha uint8
			if lightSource {
				// Formiga escura sobre fundo branco.
				switch {
				case lum >= 238:
					alpha = 0
				case lum <= 72:
					alpha = c.A
				default:
					t := (lum - 72) / (238 - 72)
					alpha = uint8(float64(c.A) * (1 - t*t))
				}
			} else {
				// Formiga clara sobre fundo preto.
				switch {
				case lum <= 22:
					alpha = 0
				case lum >= 175:
					alpha = c.A
				default:
					t := (lum - 22) / (175 - 22)
					alpha = uint8(float64(c.A) * t * t)
				}
			}
			if alpha < 4 {
				out.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: alpha})
		}
	}
	return out
}

// fitMascotTransparent encaixa a mascote num quadrado com fundo transparente.
func fitMascotTransparent(src image.Image, size int, lightSource bool) image.Image {
	if size <= 0 {
		return src
	}
	cut := stripBackground(src, lightSource)
	b := cut.Bounds()

	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	pad := float64(size) * 0.06
	inner := float64(size) - 2*pad
	sw := float64(b.Dx())
	sh := float64(b.Dy())
	scale := inner / sw
	if sh*scale > inner {
		scale = inner / sh
	}
	dw := int(sw*scale + 0.5)
	dh := int(sh*scale + 0.5)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	scaled := scaleToRectNearest(cut, dw, dh)
	sb := scaled.Bounds()
	x0 := (size - sb.Dx()) / 2
	y0 := (size - sb.Dy()) / 2
	draw.Draw(dst, image.Rect(x0, y0, x0+sb.Dx(), y0+sb.Dy()), scaled, sb.Min, draw.Over)
	return dst
}

// fitMascotSolid encaixa a mascote com fundo opaco (ícones de sistema / favicon).
func fitMascotSolid(src image.Image, size int, lightSource bool, bg color.NRGBA) image.Image {
	if size <= 0 {
		return src
	}
	cut := stripBackground(src, lightSource)
	b := cut.Bounds()

	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	pad := float64(size) * 0.08
	inner := float64(size) - 2*pad
	sw := float64(b.Dx())
	sh := float64(b.Dy())
	scale := inner / sw
	if sh*scale > inner {
		scale = inner / sh
	}
	dw := int(sw*scale + 0.5)
	dh := int(sh*scale + 0.5)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	scaled := scaleToRectNearest(cut, dw, dh)
	sb := scaled.Bounds()
	x0 := (size - sb.Dx()) / 2
	y0 := (size - sb.Dy()) / 2
	draw.Draw(dst, image.Rect(x0, y0, x0+sb.Dx(), y0+sb.Dy()), scaled, sb.Min, draw.Over)
	return dst
}

// scaleToRectNearest redimensiona em estilo pixel-art (vizinho mais próximo).
func scaleToRectNearest(src image.Image, w, h int) image.Image {
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
		if sy >= b.Max.Y {
			sy = b.Max.Y - 1
		}
		for x := 0; x < w; x++ {
			sx := b.Min.X + x*b.Dx()/w
			if sx >= b.Max.X {
				sx = b.Max.X - 1
			}
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// scaleIconNearest redimensiona quadrado com vizinho mais próximo.
func scaleIconNearest(src image.Image, size int) image.Image {
	return scaleToRectNearest(src, size, size)
}

func bgDark() color.NRGBA  { return color.NRGBA{R: 15, G: 20, B: 25, A: 255} }
func bgLight() color.NRGBA { return color.NRGBA{R: 244, G: 247, B: 252, A: 255} }
