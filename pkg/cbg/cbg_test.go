package cbg

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func makeTestPNG(t *testing.T, path string, width, height int, withAlpha bool) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			a := uint8(0xFF)
			if withAlpha && (x+y)%7 == 0 {
				a = 0x80
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x * 17) & 0xFF),
				G: uint8((y * 23) & 0xFF),
				B: uint8(((x + y) * 31) & 0xFF),
				A: a,
			})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test png: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
}

func TestV1RoundTrip(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "input.png")
	cbgPath := filepath.Join(dir, "output.cbg")
	pngOutPath := filepath.Join(dir, "output.png")

	makeTestPNG(t, pngPath, 17, 13, false)

	img, err := LoadPng(pngPath)
	if err != nil {
		t.Fatalf("LoadPng failed: %v", err)
	}
	if img.Width != 17 || img.Height != 13 {
		t.Fatalf("unexpected size: %dx%d", img.Width, img.Height)
	}

	if err := EncodeCbg(img, cbgPath); err != nil {
		t.Fatalf("EncodeCbg failed: %v", err)
	}

	ok, err := IsCbgFile(cbgPath)
	if err != nil {
		t.Fatalf("IsCbgFile failed: %v", err)
	}
	if !ok {
		t.Fatalf("IsCbgFile returned false for a CBG file")
	}

	decoded, err := DecodeCbg(cbgPath)
	if err != nil {
		t.Fatalf("DecodeCbg failed: %v", err)
	}
	if decoded.Width != img.Width || decoded.Height != img.Height {
		t.Fatalf("decoded size mismatch: got %dx%d, want %dx%d",
			decoded.Width, decoded.Height, img.Width, img.Height)
	}
	if decoded.HasAlpha != img.HasAlpha {
		t.Fatalf("alpha flag mismatch: got %v, want %v", decoded.HasAlpha, img.HasAlpha)
	}
	if !bytes.Equal(decoded.Pixels, img.Pixels) {
		t.Fatalf("pixel mismatch after round trip")
	}

	if err := SavePng(decoded, pngOutPath); err != nil {
		t.Fatalf("SavePng failed: %v", err)
	}
	if _, err := os.Stat(pngOutPath); err != nil {
		t.Fatalf("output png not created: %v", err)
	}
}

func TestV1RoundTripWithAlpha(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "input.png")
	cbgPath := filepath.Join(dir, "output.cbg")

	makeTestPNG(t, pngPath, 16, 16, true)

	img, err := LoadPng(pngPath)
	if err != nil {
		t.Fatalf("LoadPng failed: %v", err)
	}
	if !img.HasAlpha {
		t.Fatalf("expected test image to have alpha")
	}

	if err := EncodeCbg(img, cbgPath); err != nil {
		t.Fatalf("EncodeCbg failed: %v", err)
	}

	decoded, err := DecodeCbg(cbgPath)
	if err != nil {
		t.Fatalf("DecodeCbg failed: %v", err)
	}
	if !bytes.Equal(decoded.Pixels, img.Pixels) {
		t.Fatalf("pixel mismatch after alpha round trip")
	}
	if !decoded.HasAlpha {
		t.Fatalf("alpha flag lost")
	}
}

func TestIsCbgFileRejectsPNG(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "input.png")
	makeTestPNG(t, pngPath, 4, 4, false)

	ok, err := IsCbgFile(pngPath)
	if err != nil {
		t.Fatalf("IsCbgFile failed: %v", err)
	}
	if ok {
		t.Fatalf("IsCbgFile returned true for a PNG")
	}
}
