package arc

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestReadArcV1(t *testing.T) {
	entries := []Entry{
		{RelativePath: "file1.txt", Offset: 0, Size: 5},
		{RelativePath: "file2.bin", Offset: 5, Size: 3},
	}
	data := buildArcV1(entries, [][]byte{
		[]byte("hello"),
		[]byte("abc"),
	})

	info, err := parseArchive(data)
	if err != nil {
		t.Fatalf("parseArchive v1 failed: %v", err)
	}
	if info.Version != 1 {
		t.Fatalf("expected version 1, got %d", info.Version)
	}
	if len(info.Entries) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(info.Entries))
	}
	for i, e := range info.Entries {
		if e.RelativePath != entries[i].RelativePath {
			t.Fatalf("entry %d name mismatch: %q vs %q", i, e.RelativePath, entries[i].RelativePath)
		}
		if e.Offset != entries[i].Offset || e.Size != entries[i].Size {
			t.Fatalf("entry %d offset/size mismatch", i)
		}
	}
}

func TestReadArcV2(t *testing.T) {
	entries := []Entry{
		{RelativePath: "image.cbg", Offset: 0, Size: 4},
	}
	data := buildArcV2(entries, [][]byte{
		[]byte("CBG!"),
	})

	info, err := parseArchive(data)
	if err != nil {
		t.Fatalf("parseArchive v2 failed: %v", err)
	}
	if info.Version != 2 {
		t.Fatalf("expected version 2, got %d", info.Version)
	}
	if len(info.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(info.Entries))
	}
	if info.Entries[0].RelativePath != "image.cbg" {
		t.Fatalf("unexpected entry name: %q", info.Entries[0].RelativePath)
	}
}

func TestExtractArcArchive(t *testing.T) {
	tmp := t.TempDir()
	arcPath := filepath.Join(tmp, "test.arc")

	entries := []Entry{
		{RelativePath: "a/file.txt", Offset: 0, Size: 5},
		{RelativePath: "b/file.bin", Offset: 5, Size: 3},
	}
	payloads := [][]byte{[]byte("hello"), []byte("abc")}
	data := buildArcV2(entries, payloads)
	if err := os.WriteFile(arcPath, data, 0o644); err != nil {
		t.Fatalf("write arc: %v", err)
	}

	outDir := filepath.Join(tmp, "out")
	extracted, err := ExtractArcArchive(arcPath, outDir, nil)
	if err != nil {
		t.Fatalf("ExtractArcArchive failed: %v", err)
	}
	if len(extracted) != 2 {
		t.Fatalf("expected 2 extracted files, got %d", len(extracted))
	}

	for i, p := range payloads {
		got, err := os.ReadFile(extracted[i])
		if err != nil {
			t.Fatalf("read extracted %d: %v", i, err)
		}
		if !bytes.Equal(got, p) {
			t.Fatalf("extracted %d mismatch: %q vs %q", i, got, p)
		}
	}
}

func buildArcV1(entries []Entry, payloads [][]byte) []byte {
	var buf bytes.Buffer
	buf.Write(magicV1)
	binary.Write(&buf, binary.LittleEndian, uint32(len(entries)))

	for _, e := range entries {
		name := make([]byte, 16)
		copy(name, e.RelativePath)
		buf.Write(name)
		binary.Write(&buf, binary.LittleEndian, e.Offset)
		binary.Write(&buf, binary.LittleEndian, e.Size)
		buf.Write(make([]byte, 8)) // padding
	}

	for _, p := range payloads {
		buf.Write(p)
	}
	return buf.Bytes()
}

func buildArcV2(entries []Entry, payloads [][]byte) []byte {
	var buf bytes.Buffer
	buf.Write(magicV2)
	binary.Write(&buf, binary.LittleEndian, uint32(len(entries)))

	for _, e := range entries {
		name := make([]byte, 16)
		copy(name, e.RelativePath)
		buf.Write(name)
		buf.Write(make([]byte, 80)) // padding
		binary.Write(&buf, binary.LittleEndian, e.Offset)
		binary.Write(&buf, binary.LittleEndian, e.Size)
		buf.Write(make([]byte, 24)) // padding
	}

	for _, p := range payloads {
		buf.Write(p)
	}
	return buf.Bytes()
}
