// Package asset implements file type probing for BGI engine assets.
package asset

import (
	"bytes"
	"os"

	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/bse"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/audio"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/cbg"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/dsc"
)

// AssetKind identifies the detected kind of an asset file.
type AssetKind int

const (
	Unknown AssetKind = iota
	CbgImage
	DscScript
	RawCompiledScript
	BgiAudio
	PngImage
	JpegImage
	BmpImage
	OggAudio
	WavAudio
)

// AssetInfo holds the result of ProbeAsset.
type AssetInfo struct {
	Kind               AssetKind
	Label              string
	SuggestedExtension string
}

var (
	pngMagic  = []byte("\x89PNG\r\n\x1A\n")
	jpegMagic = []byte("\xFF\xD8\xFF")
	bmpMagic  = []byte("BM")
	oggMagic  = []byte("OggS")
	riffMagic = []byte("RIFF")
	waveMagic = []byte("WAVE")
)

// ReadAssetData reads a file and returns its usable payload together with its
// detected asset kind. If the file is wrapped in a BSE 1.0 container, the BSE
// layer is decrypted and stripped before detection.
func ReadAssetData(path string) ([]byte, AssetInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, makeInfo(Unknown, "not-a-file", ""), err
	}
	if info.IsDir() {
		return nil, makeInfo(Unknown, "not-a-file", ""), nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, makeInfo(Unknown, "error", ""), err
	}

	if bse.IsBseContainer(raw) {
		data, err := bse.DecryptBse(raw)
		if err != nil {
			return nil, makeInfo(Unknown, "bse-error", ""), err
		}
		return data, probeAssetBytes(data), nil
	}

	return raw, probeAssetBytes(raw), nil
}

// ProbeAsset inspects the file at path and returns its asset kind and metadata.
// BSE-wrapped files are decrypted before the inner type is recognised.
func ProbeAsset(path string) (AssetInfo, error) {
	_, info, err := ReadAssetData(path)
	return info, err
}

// ProbeAssetBytes inspects an in-memory byte slice and returns its asset kind.
// The caller is responsible for stripping any BSE layer beforehand.
func ProbeAssetBytes(data []byte) AssetInfo {
	return probeAssetBytes(data)
}

func probeAssetBytes(data []byte) AssetInfo {
	if cbg.IsCbgBytes(data) {
		return makeInfo(CbgImage, "cbg-image", ".png")
	}

	if dsc.IsDscScriptBytes(data) {
		return makeInfo(DscScript, "dsc-script", ".hazuki.txt")
	}

	if dsc.IsCompiledScriptBytes(data) {
		return makeInfo(RawCompiledScript, "compiled-script", ".hazuki.txt")
	}

	if audio.IsBgiAudioBytes(data) {
		return makeInfo(BgiAudio, "bgi-audio", ".ogg")
	}

	switch {
	case bytes.HasPrefix(data, pngMagic):
		return makeInfo(PngImage, "png-image", ".png")
	case bytes.HasPrefix(data, jpegMagic):
		return makeInfo(JpegImage, "jpeg-image", ".jpg")
	case bytes.HasPrefix(data, bmpMagic):
		return makeInfo(BmpImage, "bmp-image", ".bmp")
	case bytes.HasPrefix(data, oggMagic):
		return makeInfo(OggAudio, "ogg-audio", ".ogg")
	case len(data) >= 12 && bytes.HasPrefix(data, riffMagic) && bytes.Equal(data[8:12], waveMagic):
		return makeInfo(WavAudio, "wav-audio", ".wav")
	default:
		return makeInfo(Unknown, "unknown", "")
	}
}

// ToString returns the textual label for an AssetKind.
func ToString(kind AssetKind) string {
	switch kind {
	case CbgImage:
		return "cbg-image"
	case DscScript:
		return "dsc-script"
	case RawCompiledScript:
		return "compiled-script"
	case BgiAudio:
		return "bgi-audio"
	case PngImage:
		return "png-image"
	case JpegImage:
		return "jpeg-image"
	case BmpImage:
		return "bmp-image"
	case OggAudio:
		return "ogg-audio"
	case WavAudio:
		return "wav-audio"
	default:
		return "unknown"
	}
}

func makeInfo(kind AssetKind, label, ext string) AssetInfo {
	return AssetInfo{
		Kind:               kind,
		Label:              label,
		SuggestedExtension: ext,
	}
}
