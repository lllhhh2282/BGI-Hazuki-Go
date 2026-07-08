package dsc

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
)

const (
	dscImageHeaderSize = 16
	maxImageDimension  = 8096
)

// IsDscImage reports whether data looks like a raw image stored inside a DSC
// container. The header is 16 bytes: width(2), height(2), bpp(1), then 11 zeros.
// Supported bpp values are 8, 24 and 32.
func IsDscImage(data []byte) bool {
	if len(data) < dscImageHeaderSize {
		return false
	}

	width := uint32(data[0]) | uint32(data[1])<<8
	height := uint32(data[2]) | uint32(data[3])<<8
	bpp := data[4]

	if width == 0 || width > maxImageDimension {
		return false
	}
	if height == 0 || height > maxImageDimension {
		return false
	}
	if bpp != 8 && bpp != 24 && bpp != 32 {
		return false
	}
	for i := 5; i < dscImageHeaderSize; i++ {
		if data[i] != 0 {
			return false
		}
	}

	expectedSize := int64(width) * int64(height) * int64(bpp) / 8
	return int64(len(data)-dscImageHeaderSize) >= expectedSize
}

// SaveDscImageAsPng decodes a DSC-embedded image and writes it to basePath.png.
func SaveDscImageAsPng(data []byte, basePath string) error {
	if !IsDscImage(data) {
		return fmt.Errorf("dsc: data is not a DSC image")
	}

	width := int(data[0]) | int(data[1])<<8
	height := int(data[2]) | int(data[3])<<8
	bpp := data[4]

	rect := image.Rect(0, 0, width, height)
	out := image.NewRGBA(rect)

	src := dscImageHeaderSize
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if src >= len(data) {
				return fmt.Errorf("dsc: image data truncated at pixel (%d,%d)", x, y)
			}
			b := data[src]
			src++
			var g, r, a uint8 = b, b, 0xFF
			if bpp == 24 || bpp == 32 {
				if src+1 >= len(data) {
					return fmt.Errorf("dsc: image data truncated at pixel (%d,%d)", x, y)
				}
				g = data[src]
				src++
				r = data[src]
				src++
			}
			if bpp == 32 {
				if src >= len(data) {
					return fmt.Errorf("dsc: image data truncated at pixel (%d,%d)", x, y)
				}
				a = data[src]
				src++
			}
			out.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	outputPath := basePath
	if !strings.HasSuffix(strings.ToLower(outputPath), ".png") {
		outputPath += ".png"
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("dsc: failed to create %s: %w", outputPath, err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		return fmt.Errorf("dsc: failed to encode PNG %s: %w", outputPath, err)
	}
	return nil
}
