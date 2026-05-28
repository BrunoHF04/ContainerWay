package main

import (
	"image"
	"image/png"
	"os"
)

// writePNG grava uma imagem PNG no disco.
func writePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
