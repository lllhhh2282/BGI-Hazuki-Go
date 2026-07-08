package dsc

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestDscDecompressCompressRoundTrip(t *testing.T) {
	// Use a non-trivial compiled payload so LZ look-backs are exercised.
	compiled := make([]byte, 0, 256)
	pattern := []byte("Hello, DSC world! This is a test string for round-trip compression. ")
	for len(compiled) < 256 {
		compiled = append(compiled, pattern...)
	}
	compiled = compiled[:256]

	compressed, err := EncodeCompiledToDsc(compiled)
	if err != nil {
		t.Fatalf("EncodeCompiledToDsc failed: %v", err)
	}

	if len(compressed) < 0x220 {
		t.Fatalf("compressed output too small: %d", len(compressed))
	}
	if !bytes.Equal(compressed[:len(dscMagic)], dscMagic) {
		t.Fatalf("compressed output missing DSC magic")
	}

	decoded, err := DecodeDscToCompiled(compressed)
	if err != nil {
		t.Fatalf("DecodeDscToCompiled failed: %v", err)
	}
	if !bytes.Equal(decoded, compiled) {
		t.Fatalf("DSC round-trip mismatch: got %d bytes, want %d", len(decoded), len(compiled))
	}
}

func TestRawCompiledExtractApplyRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "scenario_01.dsc")

	compiled := buildTestRawCompiled()
	if err := os.WriteFile(scriptPath, compiled, 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	project, err := ExtractTextProject(scriptPath, 932, 932)
	if err != nil {
		t.Fatalf("ExtractTextProject failed: %v", err)
	}
	if len(project.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(project.Entries))
	}

	projectPath := filepath.Join(tmp, "scenario_01.dsc.hazuki.txt")
	if err := SaveTextProject(project, scriptPath, projectPath); err != nil {
		t.Fatalf("SaveTextProject failed: %v", err)
	}

	loaded, err := LoadTextProject(projectPath)
	if err != nil {
		t.Fatalf("LoadTextProject failed: %v", err)
	}
	if len(loaded.Entries) != len(project.Entries) {
		t.Fatalf("loaded entry count mismatch: %d vs %d", len(loaded.Entries), len(project.Entries))
	}
	for i := range project.Entries {
		if loaded.Entries[i].OriginalText != project.Entries[i].OriginalText {
			t.Fatalf("entry %d src mismatch", i)
		}
		if loaded.Entries[i].Kind != project.Entries[i].Kind {
			t.Fatalf("entry %d kind mismatch", i)
		}
	}

	// Apply without changes should reproduce the original compiled bytes.
	patchedPath := filepath.Join(tmp, "scenario_01.dsc.patched")
	if err := ApplyTextProject(projectPath, scriptPath, patchedPath, 0); err != nil {
		t.Fatalf("ApplyTextProject (no change) failed: %v", err)
	}
	patched, err := os.ReadFile(patchedPath)
	if err != nil {
		t.Fatalf("read patched: %v", err)
	}
	if !bytes.Equal(patched, compiled) {
		t.Fatalf("no-change round-trip mismatch")
	}

	// Apply with a translation.
	loaded.Entries[0].TranslationText = "Translated"
	if err := SaveTextProject(loaded, scriptPath, projectPath); err != nil {
		t.Fatalf("SaveTextProject (translated) failed: %v", err)
	}
	if err := ApplyTextProject(projectPath, scriptPath, patchedPath, 0); err != nil {
		t.Fatalf("ApplyTextProject (translated) failed: %v", err)
	}
	reloadedLayout, err := LoadScriptLayout(patchedPath)
	if err != nil {
		t.Fatalf("LoadScriptLayout (patched) failed: %v", err)
	}
	if len(reloadedLayout.TextSlots) != 2 {
		t.Fatalf("patched text slot count mismatch: %d", len(reloadedLayout.TextSlots))
	}
	if string(reloadedLayout.TextSlots[0].Bytes) != "Translated" {
		t.Fatalf("patched text mismatch: %q", string(reloadedLayout.TextSlots[0].Bytes))
	}
	if string(reloadedLayout.TextSlots[1].Bytes) != "World" {
		t.Fatalf("patched second slot mismatch: %q", string(reloadedLayout.TextSlots[1].Bytes))
	}
}

func TestProjectTextRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.hazuki.txt")

	project := &TextProject{
		ContainerKind:  DscCompressed,
		DecodeCodepage: 932,
		EncodeCodepage: 936,
		Entries: []TextEntry{
			{
				Index:           1,
				TextOffset:      0x12,
				Kind:            Name,
				Comment:         "NAME",
				CodeOffsets:     []uint32{0x420, 0x42C},
				OriginalText:    "佐伯",
				TranslationText: "Sae",
			},
			{
				Index:        2,
				TextOffset:   0x30,
				Kind:         Text,
				Comment:      "TEXT",
				CodeOffsets:  []uint32{0x500},
				OriginalText: "Line1\nLine2\tTab",
			},
		},
	}

	if err := SaveTextProject(project, "scenario_01.dsc", path); err != nil {
		t.Fatalf("SaveTextProject failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project: %v", err)
	}
	if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
		t.Fatalf("missing UTF-8 BOM")
	}

	loaded, err := LoadTextProject(path)
	if err != nil {
		t.Fatalf("LoadTextProject failed: %v", err)
	}
	if loaded.ContainerKind != project.ContainerKind {
		t.Fatalf("container kind mismatch")
	}
	if loaded.DecodeCodepage != project.DecodeCodepage || loaded.EncodeCodepage != project.EncodeCodepage {
		t.Fatalf("codepage mismatch")
	}
	if len(loaded.Entries) != len(project.Entries) {
		t.Fatalf("entry count mismatch")
	}
	for i, want := range project.Entries {
		got := loaded.Entries[i]
		if got.TextOffset != want.TextOffset || got.Kind != want.Kind {
			t.Fatalf("entry %d metadata mismatch", i)
		}
		if got.OriginalText != want.OriginalText {
			t.Fatalf("entry %d src mismatch: %q vs %q", i, got.OriginalText, want.OriginalText)
		}
		if got.TranslationText != want.TranslationText {
			t.Fatalf("entry %d dst mismatch", i)
		}
		if len(got.CodeOffsets) != len(want.CodeOffsets) {
			t.Fatalf("entry %d code offset count mismatch", i)
		}
		for j := range want.CodeOffsets {
			if got.CodeOffsets[j] != want.CodeOffsets[j] {
				t.Fatalf("entry %d code offset %d mismatch", i, j)
			}
		}
	}
}

func TestEscapeUnescape(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{"hello", "hello"},
		{"a\\b", "a\\\\b"},
		{"a\nb", "a\\nb"},
		{"a\rb", "a\\rb"},
		{"a\tb", "a\\tb"},
	}
	for _, c := range cases {
		escaped := EscapeProjectText(c.in)
		if escaped != c.out {
			t.Errorf("EscapeProjectText(%q) = %q, want %q", c.in, escaped, c.out)
		}
		unescaped := UnescapeProjectText(escaped)
		if unescaped != c.in {
			t.Errorf("UnescapeProjectText(%q) = %q, want %q", escaped, unescaped, c.in)
		}
	}
}

func TestScriptConfigDetection(t *testing.T) {
	if getScriptConfig([]byte("not compiled")).HeaderSize != 0 {
		t.Fatalf("non-compiled should use ver000")
	}
	if getScriptConfig(compiledMagic).HeaderSize != 0x1C {
		t.Fatalf("compiled magic should use ver100")
	}
}

func TestCodepageRoundTrip(t *testing.T) {
	// Japanese text encoded as CP932 and decoded back.
	original := "佐伯"
	b, err := EncodeScriptText(original, 932)
	if err != nil {
		t.Fatalf("EncodeScriptText failed: %v", err)
	}
	decoded, err := DecodeScriptBytes(b, 932)
	if err != nil {
		t.Fatalf("DecodeScriptBytes failed: %v", err)
	}
	if decoded != original {
		t.Fatalf("codepage round-trip mismatch: %q vs %q", decoded, original)
	}
}

func TestHexEscapeRoundTrip(t *testing.T) {
	// A byte that cannot be decoded as ShiftJIS should be preserved as &#XXXX.
	b := []byte{0xFF}
	decoded, err := DecodeScriptBytes(b, 932)
	if err != nil {
		t.Fatalf("DecodeScriptBytes failed: %v", err)
	}
	if decoded != "&#00FF" {
		t.Fatalf("unexpected escape: %q", decoded)
	}
	encoded, err := EncodeScriptText(decoded, 932)
	if err != nil {
		t.Fatalf("EncodeScriptText failed: %v", err)
	}
	if !bytes.Equal(encoded, b) {
		t.Fatalf("hex escape round-trip mismatch: %v vs %v", encoded, b)
	}
}

func TestContainerDetection(t *testing.T) {
	tmp := t.TempDir()

	dscPath := filepath.Join(tmp, "test.dsc")
	os.WriteFile(dscPath, append(dscMagic, make([]byte, 16)...), 0o644)
	ok, err := IsDscScript(dscPath)
	if err != nil || !ok {
		t.Fatalf("IsDscScript failed: %v %v", ok, err)
	}

	compiledPath := filepath.Join(tmp, "test.c")
	os.WriteFile(compiledPath, compiledMagic, 0o644)
	ok, err = IsCompiledScript(compiledPath)
	if err != nil || !ok {
		t.Fatalf("IsCompiledScript failed: %v %v", ok, err)
	}
}

func TestDscImageDetectionAndSave(t *testing.T) {
	tmp := t.TempDir()

	// 2x2 24bpp image: 16-byte header + 12 bytes of BGR data.
	data := make([]byte, 16+2*2*3)
	data[0] = 2 // width
	data[1] = 0
	data[2] = 2 // height
	data[3] = 0
	data[4] = 24
	for i := 5; i < 16; i++ {
		data[i] = 0
	}
	idx := 16
	for i := 0; i < 4; i++ {
		data[idx] = byte(i)       // B
		data[idx+1] = byte(i + 1) // G
		data[idx+2] = byte(i + 2) // R
		idx += 3
	}

	if !IsDscImage(data) {
		t.Fatal("IsDscImage did not recognise valid DSC image header")
	}

	basePath := filepath.Join(tmp, "dsc_image")
	if err := SaveDscImageAsPng(data, basePath); err != nil {
		t.Fatalf("SaveDscImageAsPng failed: %v", err)
	}

	pngPath := basePath + ".png"
	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatalf("open saved PNG: %v", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode saved PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Fatalf("unexpected image size: %dx%d", bounds.Dx(), bounds.Dy())
	}
}

// buildTestRawCompiled constructs a minimal Ver1.00 raw compiled script with two text slots.
func buildTestRawCompiled() []byte {
	// Ver100: header_size is 0x1C. The raw compiled magic is 27 chars + null (28 bytes).
	// header_bytes are the first 28 bytes, so the trailing null is the first code byte.
	header := append([]byte("BurikoCompiledScriptVer1.00"), 0x00)
	if len(header) != 0x1C {
		panic("unexpected header length")
	}

	code := make([]byte, 0)
	// Additional header value at offset 0x1C relative to header start.
	code = append(code, 0x00, 0x00, 0x00, 0x00)
	// First string opcode at offset 0x20.
	code = append(code, 0x03, 0x00, 0x00, 0x00)
	// Pointer to text slot 0; computed after text_boundary is known.
	pointer0Pos := len(code)
	code = append(code, 0x00, 0x00, 0x00, 0x00)
	// Text function 0x140 at offset text_pos=+4 from pointer.
	code = append(code, 0x40, 0x01, 0x00, 0x00)
	// Second string opcode.
	code = append(code, 0x03, 0x00, 0x00, 0x00)
	// Pointer to text slot 1; computed after text_boundary is known.
	pointer1Pos := len(code)
	code = append(code, 0x00, 0x00, 0x00, 0x00)
	// Text function 0x140 at offset text_pos=+4 from pointer.
	code = append(code, 0x40, 0x01, 0x00, 0x00)
	// Text boundary marker.
	code = append(code, 0x1B, 0x00, 0x00, 0x00)

	textBoundary := len(header) + len(code)
	codeSize := textBoundary - len(header)
	writeU32LE(code, pointer0Pos, uint32(codeSize))
	writeU32LE(code, pointer1Pos, uint32(codeSize)+6)

	text := []byte("Hello\x00World\x00")
	compiled := make([]byte, 0)
	compiled = append(compiled, header...)
	compiled = append(compiled, code...)
	compiled = append(compiled, text...)
	return compiled
}
