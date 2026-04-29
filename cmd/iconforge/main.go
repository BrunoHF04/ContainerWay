package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// main inicializa e executa o fluxo principal deste binario.
func main() {
	const size = 256
	icon := buildIcon(size)

	if err := os.MkdirAll("assets", 0o755); err != nil {
		panic(err)
	}
	if err := writePNG(icon, filepath.Join("assets", "containerway-icon.png")); err != nil {
		panic(err)
	}
	if err := writeICO(icon, filepath.Join("assets", "containerway-icon.ico")); err != nil {
		panic(err)
	}
}

// writePNG executa parte da logica deste modulo.
func writePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writeICO executa parte da logica deste modulo.
func writeICO(img image.Image, path string) error {
	var pngBuf []byte
	{
		tmp := new(bytesBuffer)
		if err := png.Encode(tmp, img); err != nil {
			return err
		}
		pngBuf = tmp.Bytes()
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// ICO header
	if err := binary.Write(f, binary.LittleEndian, uint16(0)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}

	// ICONDIRENTRY
	width := uint8(0)  // 0 => 256
	height := uint8(0) // 0 => 256
	if err := binary.Write(f, binary.LittleEndian, width); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, height); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint8(0)); err != nil { // color count
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint8(0)); err != nil { // reserved
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // planes
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(32)); err != nil { // bit count
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(len(pngBuf))); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(6+16)); err != nil { // data offset
		return err
	}

	_, err = f.Write(pngBuf)
	return err
}

// buildIcon executa parte da logica deste modulo.
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

// drawArrow executa parte da logica deste modulo.
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

// drawRoundedRect executa parte da logica deste modulo.
func drawRoundedRect(img *image.NRGBA, x, y, w, h, r float64, c color.NRGBA) {
	for yy := int(y); yy < int(y+h); yy++ {
		for xx := int(x); xx < int(x+w); xx++ {
			if insideRoundedRect(float64(xx)+0.5, float64(yy)+0.5, x, y, w, h, r) {
				setPixelBlend(img, xx, yy, c)
			}
		}
	}
}

// insideRoundedRect executa parte da logica deste modulo.
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

// setPixelBlend executa parte da logica deste modulo.
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

// lerp executa parte da logica deste modulo.
func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	t = clamp01(t)
	return color.NRGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

// clamp01 executa parte da logica deste modulo.
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

// Write executa parte da logica deste modulo.
func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// Bytes executa parte da logica deste modulo.
func (b *bytesBuffer) Bytes() []byte {
	return b.buf
}
