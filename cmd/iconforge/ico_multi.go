package main

import (
	"encoding/binary"
	"image"
	"image/png"
	"os"
)

// writeICOMulti grava um .ico com vários tamanhos PNG embutidos.
func writeICOMulti(path string, sizes []int, source image.Image) error {
	type entry struct {
		size int
		png  []byte
	}
	var entries []entry
	for _, sz := range sizes {
		scaled := scaleIcon(source, sz)
		tmp := new(bytesBuffer)
		if err := png.Encode(tmp, scaled); err != nil {
			return err
		}
		entries = append(entries, entry{size: sz, png: tmp.Bytes()})
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	n := uint16(len(entries))
	if err := binary.Write(f, binary.LittleEndian, uint16(0)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, n); err != nil {
		return err
	}

	offset := uint32(6 + 16*int(n))
	for _, e := range entries {
		w, h := icoDimension(e.size)
		if err := binary.Write(f, binary.LittleEndian, w); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, h); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, uint8(0)); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, uint8(0)); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, uint16(32)); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, uint32(len(e.png))); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, offset); err != nil {
			return err
		}
		offset += uint32(len(e.png))
	}
	for _, e := range entries {
		if _, err := f.Write(e.png); err != nil {
			return err
		}
	}
	return nil
}

// icoDimension converte tamanho em bytes de largura/altura do ICO.
func icoDimension(size int) (uint8, uint8) {
	if size >= 256 {
		return 0, 0
	}
	return uint8(size), uint8(size)
}
