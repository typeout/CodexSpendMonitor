package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: iconify input.png output.ico")
		os.Exit(2)
	}

	srcFile, err := os.Open(os.Args[1])
	if err != nil {
		fatal(err)
	}
	defer srcFile.Close()

	src, _, err := image.Decode(srcFile)
	if err != nil {
		fatal(err)
	}

	sizes := []int{256, 64, 48, 32, 16}
	images := make([][]byte, 0, len(sizes))
	for _, size := range sizes {
		tmp, err := os.CreateTemp("", "codex-icon-*.png")
		if err != nil {
			fatal(err)
		}
		resized := resizeNearest(src, size)
		if err := png.Encode(tmp, resized); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			fatal(err)
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmp.Name())
			fatal(err)
		}
		data, err := os.ReadFile(tmp.Name())
		_ = os.Remove(tmp.Name())
		if err != nil {
			fatal(err)
		}
		images = append(images, data)
	}

	out, err := os.Create(os.Args[2])
	if err != nil {
		fatal(err)
	}
	defer out.Close()

	write16(out, 0)
	write16(out, 1)
	write16(out, uint16(len(sizes)))

	offset := uint32(6 + len(sizes)*16)
	for i, size := range sizes {
		width := byte(size)
		if size == 256 {
			width = 0
		}
		out.Write([]byte{width, width, 0, 0})
		write16(out, 1)
		write16(out, 32)
		write32(out, uint32(len(images[i])))
		write32(out, offset)
		offset += uint32(len(images[i]))
	}
	for _, data := range images {
		if _, err := out.Write(data); err != nil {
			fatal(err)
		}
	}
}

func resizeNearest(src image.Image, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	bounds := src.Bounds()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			srcX := bounds.Min.X + x*bounds.Dx()/size
			srcY := bounds.Min.Y + y*bounds.Dy()/size
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func write16(file *os.File, value uint16) {
	if err := binary.Write(file, binary.LittleEndian, value); err != nil {
		fatal(err)
	}
}

func write32(file *os.File, value uint32) {
	if err := binary.Write(file, binary.LittleEndian, value); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
