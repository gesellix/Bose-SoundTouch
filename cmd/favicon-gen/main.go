// Package main provides a utility to generate PNG and ICO favicons from SVG source files.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func main() {
	mediaDir := "pkg/service/handlers/web/img"
	files := []string{"favicon-braille", "favicon-morse"}

	for _, name := range files {
		svgPath := filepath.Join(mediaDir, name+".svg")
		pngPath := filepath.Join(mediaDir, name+".png")
		icoPath := filepath.Join(mediaDir, name+".ico")

		fmt.Printf("Processing %s...\n", name)

		// 1. Render SVG to PNG
		img, err := renderSVG(svgPath, 32, 32)
		if err != nil {
			log.Fatalf("Failed to render %s: %v", svgPath, err)
		}

		f, err := os.Create(pngPath)
		if err != nil {
			log.Fatalf("Failed to create %s: %v", pngPath, err)
		}

		if err := png.Encode(f, img); err != nil {
			f.Close()
			log.Fatalf("Failed to encode PNG %s: %v", pngPath, err)
		}

		f.Close()
		fmt.Printf("Created %s\n", pngPath)

		// 2. Create ICO (containing multiple sizes)
		sizes := []int{16, 32, 48}

		var images []image.Image

		for _, s := range sizes {
			m, err := renderSVG(svgPath, s, s)
			if err != nil {
				log.Fatalf("Failed to render %s at size %d: %v", svgPath, s, err)
			}

			images = append(images, m)
		}

		if err := writeICO(icoPath, images); err != nil {
			log.Fatalf("Failed to write ICO %s: %v", icoPath, err)
		}

		fmt.Printf("Created %s\n", icoPath)
	}
}

func renderSVG(path string, w, h int) (image.Image, error) {
	in, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	icon, err := oksvg.ReadIconStream(in)
	if err != nil {
		return nil, err
	}

	icon.SetTarget(0, 0, float64(w), float64(h))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	gv := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	dasher := rasterx.NewDasher(w, h, gv)
	icon.Draw(dasher, 1.0)

	return rgba, nil
}

// Simple ICO encoder that wraps PNGs
func writeICO(path string, images []image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	defer bw.Flush()

	// ICONDIR header
	// Reserved (2), Type (2), Count (2)
	binary.Write(bw, binary.LittleEndian, uint16(0))
	binary.Write(bw, binary.LittleEndian, uint16(1)) // 1 = ICO
	binary.Write(bw, binary.LittleEndian, uint16(len(images)))

	var pngData [][]byte

	for _, img := range images {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return err
		}

		pngData = append(pngData, buf.Bytes())
	}

	offset := uint32(6 + len(images)*16)
	for i, img := range images {
		b := img.Bounds()

		width := uint8(b.Dx())
		if b.Dx() >= 256 {
			width = 0
		}

		height := uint8(b.Dy())
		if b.Dy() >= 256 {
			height = 0
		}

		// ICONDIRENTRY
		bw.WriteByte(width)
		bw.WriteByte(height)
		bw.WriteByte(0)                                   // Color count
		bw.WriteByte(0)                                   // Reserved
		binary.Write(bw, binary.LittleEndian, uint16(1))  // Planes (1)
		binary.Write(bw, binary.LittleEndian, uint16(32)) // Bits per pixel (32)
		binary.Write(bw, binary.LittleEndian, uint32(len(pngData[i])))
		binary.Write(bw, binary.LittleEndian, offset)

		offset += uint32(len(pngData[i]))
	}

	for _, data := range pngData {
		bw.Write(data)
	}

	return nil
}
