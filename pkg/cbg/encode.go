package cbg

import (
	"bytes"
	"fmt"
	"os"

	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/binutil"
	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/codec"
)

// EncodeCbg encodes a BGRA32 RasterImage as a CBG v1 file.
func EncodeCbg(img *RasterImage, path string) error {
	if img == nil {
		return fmt.Errorf("cbg: nil image")
	}
	expected := int64(img.Width) * int64(img.Height) * 4
	if int64(len(img.Pixels)) != expected {
		return fmt.Errorf("cbg: pixel buffer size %d does not match %dx%dx4", len(img.Pixels), img.Width, img.Height)
	}
	if img.Width > 0xFFFF || img.Height > 0xFFFF {
		return fmt.Errorf("cbg v1: dimensions must be <= 65535")
	}

	hasRealAlpha := false
	for i := 3; i < len(img.Pixels); i += 4 {
		if img.Pixels[i] != 0xFF {
			hasRealAlpha = true
			break
		}
	}

	channels := uint32(3)
	if hasRealAlpha {
		channels = 4
	}

	packed := packPixels(img, channels)
	applyAverageSampling(packed, img.Width, img.Height, channels)
	rleData := codec.EncodeRLE(packed)

	var frequencies [256]uint32
	for _, v := range rleData {
		frequencies[v]++
	}

	tree := buildHuffmanTree(frequencies)
	if tree == nil {
		return fmt.Errorf("cbg v1: failed to build huffman tree")
	}

	weightPlain := serializeFrequencies(frequencies)
	weightEncrypted := make([]byte, len(weightPlain))
	copy(weightEncrypted, weightPlain)
	const key uint32 = 0
	weightSum, weightXor := encrypt(weightEncrypted, key)
	bitstream := huffmanEncode(rleData, tree)

	hdr := &cbgHeader{
		Width:              uint16(img.Width),
		Height:             uint16(img.Height),
		BitsPerPixel:       channels * 8,
		IntermediateLength: uint32(len(rleData)),
		Key:                key,
		EncLength:          uint32(len(weightPlain)),
		ChecksumSum:        weightSum,
		ChecksumXor:        weightXor,
		Version:            1,
	}

	buf := &bytes.Buffer{}
	writeHeader(buf, hdr)
	buf.Write(weightEncrypted)
	buf.Write(bitstream)

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("cbg: failed to write %s: %w", path, err)
	}
	return nil
}

func packPixels(img *RasterImage, channels uint32) []byte {
	packed := make([]byte, 0, int(img.Width)*int(img.Height)*int(channels))
	for i := 0; i < len(img.Pixels); i += 4 {
		packed = append(packed, img.Pixels[i+0])
		if channels >= 2 {
			packed = append(packed, img.Pixels[i+1])
			packed = append(packed, img.Pixels[i+2])
		}
		if channels == 4 {
			packed = append(packed, img.Pixels[i+3])
		}
	}
	return packed
}

func applyAverageSampling(pixels []byte, width, height, channels uint32) {
	if len(pixels) == 0 {
		return
	}
	stride := int(width) * int(channels)
	for y := int(height) - 1; y >= 0; y-- {
		rowOffset := y * stride
		for x := int(width) - 1; x >= 0; x-- {
			index := rowOffset + x*int(channels)
			for c := int(channels) - 1; c >= 0; c-- {
				average := 0
				if x > 0 {
					average += int(pixels[index+c-int(channels)])
				}
				if y > 0 {
					average += int(pixels[index+c-stride])
				}
				if x > 0 && y > 0 {
					average /= 2
				}
				if average != 0 {
					pixels[index+c] -= byte(average)
				}
			}
		}
	}
}

func serializeFrequencies(frequencies [256]uint32) []byte {
	buf := &bytes.Buffer{}
	for _, f := range frequencies {
		binutil.WriteVarInt(buf, f)
	}
	return buf.Bytes()
}
