package cbg

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

// LoadPng reads a PNG file and converts it to a BGRA32 RasterImage.
func LoadPng(path string) (*RasterImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cbg: failed to open %s: %w", path, err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("cbg: failed to decode PNG %s: %w", path, err)
	}

	bounds := img.Bounds()
	width := uint32(bounds.Dx())
	height := uint32(bounds.Dy())
	out := &RasterImage{
		Width:  width,
		Height: height,
		Pixels: make([]uint8, width*height*4),
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			nrgba := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			dst := uint32(y-bounds.Min.Y)*width*4 + uint32(x-bounds.Min.X)*4
			out.Pixels[dst+0] = nrgba.B
			out.Pixels[dst+1] = nrgba.G
			out.Pixels[dst+2] = nrgba.R
			out.Pixels[dst+3] = nrgba.A
			if nrgba.A != 0xFF {
				out.HasAlpha = true
			}
		}
	}
	return out, nil
}

// SavePng saves a BGRA32 RasterImage as a PNG file.
func SavePng(img *RasterImage, path string) error {
	if img == nil {
		return fmt.Errorf("cbg: nil image")
	}
	expected := int64(img.Width) * int64(img.Height) * 4
	if int64(len(img.Pixels)) != expected {
		return fmt.Errorf("cbg: pixel buffer size %d does not match %dx%dx4", len(img.Pixels), img.Width, img.Height)
	}

	rect := image.Rect(0, 0, int(img.Width), int(img.Height))
	out := image.NewRGBA(rect)
	for y := 0; y < int(img.Height); y++ {
		for x := 0; x < int(img.Width); x++ {
			src := uint32(y)*img.Width*4 + uint32(x)*4
			c := color.RGBA{
				R: img.Pixels[src+2],
				G: img.Pixels[src+1],
				B: img.Pixels[src+0],
				A: img.Pixels[src+3],
			}
			out.SetRGBA(x, y, c)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("cbg: failed to create %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		return fmt.Errorf("cbg: failed to encode PNG %s: %w", path, err)
	}
	return nil
}
