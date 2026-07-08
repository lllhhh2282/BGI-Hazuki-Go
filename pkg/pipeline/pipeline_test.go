package pipeline

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func TestRunFullUnpackPipeline(t *testing.T) {
	dir := t.TempDir()
	arcPath := filepath.Join(dir, "data.arc")

	// Create a synthetic ARC containing one BGI audio file.
	audioName := "bgm.bw"
	audioData := makeBgiAudio([]byte("OggS fake audio"))
	if err := writeArc(arcPath, map[string][]byte{audioName: audioData}); err != nil {
		t.Fatalf("write arc: %v", err)
	}

	opts := Options{
		UnpackRoot:     filepath.Join(dir, "unpack"),
		DecodeCodepage: 932,
		EncodeCodepage: 932,
	}

	var logs []string
	var lastProgress Progress
	cb := Callbacks{
		OnLog: func(line string) { logs = append(logs, line) },
		OnProgress: func(p Progress) { lastProgress = p },
	}

	result, err := RunFullUnpackPipeline([]string{arcPath}, opts, cb)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	if result.ArchiveCount != 1 {
		t.Fatalf("archive count: got %d, want 1", result.ArchiveCount)
	}
	if result.ExtractedCount != 1 {
		t.Fatalf("extracted count: got %d, want 1", result.ExtractedCount)
	}
	if result.ProcessedCount != 1 {
		t.Fatalf("processed count: got %d, want 1", result.ProcessedCount)
	}
	if result.ConvertedCount != 1 {
		t.Fatalf("converted count: got %d, want 1", result.ConvertedCount)
	}
	if result.SkippedCount != 0 {
		t.Fatalf("skipped count: got %d, want 0", result.SkippedCount)
	}
	if lastProgress.Completed != lastProgress.Total || lastProgress.Total == 0 {
		t.Fatalf("progress did not complete: %+v", lastProgress)
	}

	oggPath := filepath.Join(result.UnpackRoot, "data", audioName+".ogg")
	if _, err := os.Stat(oggPath); err != nil {
		t.Fatalf("expected ogg file %s: %v", oggPath, err)
	}
}

func TestRunFullUnpackPipelineNoInputs(t *testing.T) {
	_, err := RunFullUnpackPipeline([]string{}, Options{}, Callbacks{})
	if err == nil {
		t.Fatalf("expected error for empty inputs")
	}
}

func TestRunFullUnpackPipelineNoArcFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := RunFullUnpackPipeline([]string{dir}, Options{}, Callbacks{})
	if err == nil {
		t.Fatalf("expected error when no arc files found")
	}
}

func makeBgiAudio(payload []byte) []byte {
	const headerSize uint32 = 12
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, headerSize)
	buf.WriteString("bw  ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(payload)))
	buf.Write(payload)
	return buf.Bytes()
}

func writeArc(path string, files map[string][]byte) error {
	const headerSize = 16
	const entrySize = 128
	const nameSize = 96

	entryCount := uint32(len(files))

	buf := &bytes.Buffer{}
	buf.WriteString("BURIKO ARC20")
	_ = binary.Write(buf, binary.LittleEndian, entryCount)

	// Sort names for deterministic output.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	offsets := make(map[string]uint32)
	sizes := make(map[string]uint32)
	curOffset := uint32(0)
	for _, name := range names {
		offsets[name] = curOffset
		sizes[name] = uint32(len(files[name]))
		curOffset += uint32(len(files[name]))
	}

	for _, name := range names {
		entry := make([]byte, entrySize)
		encoded, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(name))
		if err != nil {
			return err
		}
		copy(entry[:nameSize], encoded)
		binary.LittleEndian.PutUint32(entry[nameSize:nameSize+4], offsets[name])
		binary.LittleEndian.PutUint32(entry[nameSize+4:nameSize+8], sizes[name])
		buf.Write(entry)
	}

	for _, name := range names {
		buf.Write(files[name])
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
