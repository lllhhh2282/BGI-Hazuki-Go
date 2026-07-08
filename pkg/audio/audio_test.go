package audio

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestIsBgiAudioFile(t *testing.T) {
	dir := t.TempDir()

	// Valid BGI audio container.
	valid := filepath.Join(dir, "bgm.bw")
	if err := writeBgiAudio(valid, []byte("OGG")); err != nil {
		t.Fatalf("write valid audio: %v", err)
	}
	ok, err := IsBgiAudioFile(valid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected valid BGI audio")
	}

	// Too small file.
	small := filepath.Join(dir, "small.bw")
	if err := os.WriteFile(small, []byte("bw"), 0o644); err != nil {
		t.Fatalf("write small file: %v", err)
	}
	ok, err = IsBgiAudioFile(small)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected small file to be rejected")
	}

	// Wrong magic.
	wrong := filepath.Join(dir, "wrong.bw")
	if err := writeBgiAudio(wrong, []byte("OGG")); err != nil {
		t.Fatalf("write wrong audio: %v", err)
	}
	data, _ := os.ReadFile(wrong)
	data[4] = 'x'
	os.WriteFile(wrong, data, 0o644)
	ok, err = IsBgiAudioFile(wrong)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected wrong magic to be rejected")
	}
}

func TestExtractBgiAudioToOgg(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "bgm.bw")
	payload := []byte("OggS fake ogg data")
	if err := writeBgiAudio(input, payload); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	output := filepath.Join(dir, "bgm.ogg")
	if err := ExtractBgiAudioToOgg(input, output); err != nil {
		t.Fatalf("extract audio: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}
}

func writeBgiAudio(path string, payload []byte) error {
	// header_size is at least 12 so the payload starts immediately after
	// the fixed header fields.
	const headerSize uint32 = 12
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, headerSize)
	buf.WriteString("bw  ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(payload)))
	buf.Write(payload)
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
