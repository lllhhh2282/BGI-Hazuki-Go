package asset

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/bse"
)

func TestProbeAsset(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		prepare  func(string) error
		wantKind AssetKind
		wantExt  string
	}{
		{
			name: "png",
			prepare: func(path string) error {
				return os.WriteFile(path, []byte("\x89PNG\r\n\x1A\nrest"), 0o644)
			},
			wantKind: PngImage,
			wantExt:  ".png",
		},
		{
			name: "jpeg",
			prepare: func(path string) error {
				return os.WriteFile(path, []byte("\xFF\xD8\xFFrest"), 0o644)
			},
			wantKind: JpegImage,
			wantExt:  ".jpg",
		},
		{
			name: "bmp",
			prepare: func(path string) error {
				return os.WriteFile(path, []byte("BMrest"), 0o644)
			},
			wantKind: BmpImage,
			wantExt:  ".bmp",
		},
		{
			name: "ogg",
			prepare: func(path string) error {
				return os.WriteFile(path, []byte("OggSrest"), 0o644)
			},
			wantKind: OggAudio,
			wantExt:  ".ogg",
		},
		{
			name: "wav",
			prepare: func(path string) error {
				buf := append([]byte("RIFFxxxxWAVE"), []byte("rest")...)
				return os.WriteFile(path, buf, 0o644)
			},
			wantKind: WavAudio,
			wantExt:  ".wav",
		},
		{
			name: "bgi-audio",
			prepare: func(path string) error {
				return writeBgiAudio(path)
			},
			wantKind: BgiAudio,
			wantExt:  ".ogg",
		},
		{
			name: "unknown",
			prepare: func(path string) error {
				return os.WriteFile(path, []byte("hello world"), 0o644)
			},
			wantKind: Unknown,
			wantExt:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := tc.prepare(path); err != nil {
				t.Fatalf("prepare: %v", err)
			}
			info, err := ProbeAsset(path)
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if info.Kind != tc.wantKind {
				t.Fatalf("kind: got %d, want %d", info.Kind, tc.wantKind)
			}
			if info.SuggestedExtension != tc.wantExt {
				t.Fatalf("extension: got %q, want %q", info.SuggestedExtension, tc.wantExt)
			}
		})
	}
}

func TestToString(t *testing.T) {
	if got := ToString(CbgImage); got != "cbg-image" {
		t.Fatalf("unexpected label: %q", got)
	}
	if got := ToString(AssetKind(999)); got != "unknown" {
		t.Fatalf("unexpected label for unknown kind: %q", got)
	}
}

func writeBgiAudio(path string) error {
	const headerSize uint32 = 12
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, headerSize)
	buf.WriteString("bw  ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(4))
	buf.WriteString("OGGV")
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func TestReadAssetDataBseWrapped(t *testing.T) {
	dir := t.TempDir()

	cbgMagic := []byte{
		0x43, 0x6F, 0x6D, 0x70, 0x72, 0x65, 0x73, 0x73,
		0x65, 0x64, 0x42, 0x47, 0x5F, 0x5F, 0x5F, 0x00,
	}
	inner := append(cbgMagic, make([]byte, 48)...)
	wrapped, err := bse.EncryptBse(inner, 0xDEADBEEF)
	if err != nil {
		t.Fatalf("EncryptBse failed: %v", err)
	}

	path := filepath.Join(dir, "wrapped.cbg")
	if err := os.WriteFile(path, wrapped, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	data, info, err := ReadAssetData(path)
	if err != nil {
		t.Fatalf("ReadAssetData failed: %v", err)
	}
	if info.Kind != CbgImage {
		t.Fatalf("expected cbg-image, got %s", info.Label)
	}
	if !bytes.Equal(data, inner) {
		t.Fatal("ReadAssetData returned data that does not match decrypted inner payload")
	}

	probeInfo, err := ProbeAsset(path)
	if err != nil {
		t.Fatalf("ProbeAsset failed: %v", err)
	}
	if probeInfo.Kind != CbgImage {
		t.Fatalf("ProbeAsset expected cbg-image, got %s", probeInfo.Label)
	}
}
