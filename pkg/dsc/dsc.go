// Package dsc implements encoding/decoding and text extraction for BGI DSC scripts.
package dsc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// ScriptContainerKind identifies how a script is stored on disk.
type ScriptContainerKind int

const (
	DscCompressed ScriptContainerKind = iota
	RawCompiled
)

// TextKind classifies a string reference inside the script code.
type TextKind int

const (
	Name TextKind = iota
	Text
	RubyKanji
	RubyFurigana
	Backlog
	File
	Other
)

// TextEntry describes one translatable string slot.
type TextEntry struct {
	Index           uint32
	TextOffset      uint32
	Kind            TextKind
	Comment         string
	CodeOffsets     []uint32
	OriginalBytes   []byte
	OriginalText    string
	TranslationText string
}

// TextProject is the in-memory representation of a .hazuki.txt project.
type TextProject struct {
	ContainerKind  ScriptContainerKind
	DecodeCodepage uint32
	EncodeCodepage uint32
	Entries        []TextEntry
}

const (
	projectSuffix   = ".hazuki.txt"
	projectMagic    = "# BGI_HAZUKI_DSC_TEXT_V1"
	defaultCodepage = 932
)

var (
	// dscMagic is the first 16 bytes of a DSC container file.
	dscMagic = []byte{
		0x44, 0x53, 0x43, 0x20, 0x46, 0x4F, 0x52, 0x4D,
		0x41, 0x54, 0x20, 0x31, 0x2E, 0x30, 0x30, 0x00,
	}
	// compiledMagic is the null-terminated header of a raw compiled script.
	compiledMagic = []byte("BurikoCompiledScriptVer1.00\x00")
)

var errNotDscScript = errors.New("not a DSC script")

// IsDscScript reports whether path points to a DSC container file.
func IsDscScript(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return IsDscScriptBytes(data), nil
}

// IsDscScriptBytes reports whether data starts with the DSC magic.
func IsDscScriptBytes(data []byte) bool {
	return len(data) >= len(dscMagic) && bytes.Equal(data[:len(dscMagic)], dscMagic)
}

// IsCompiledScript reports whether path points to a raw compiled script.
func IsCompiledScript(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return IsCompiledScriptBytes(data), nil
}

// IsCompiledScriptBytes reports whether data starts with the compiled script magic.
func IsCompiledScriptBytes(data []byte) bool {
	return bytes.HasPrefix(data, compiledMagic)
}

// GetDefaultProjectPath returns the default .hazuki.txt path for a script.
func GetDefaultProjectPath(scriptPath string) string {
	return scriptPath + projectSuffix
}

// InferScriptPathFromProject strips the .hazuki.txt suffix from a project path.
// It returns an empty string if the suffix is missing.
func InferScriptPathFromProject(projectPath string) string {
	lower := strings.ToLower(projectPath)
	if !strings.HasSuffix(lower, ".hazuki.txt") {
		return ""
	}
	return projectPath[:len(projectPath)-len(projectSuffix)]
}

// GetDefaultPatchedPath returns the default patched output path for a script.
func GetDefaultPatchedPath(scriptPath string) string {
	return scriptPath + ".patched"
}

// ToString returns the textual kind name used in project files.
func ToString(kind TextKind) string {
	switch kind {
	case Name:
		return "name"
	case Text:
		return "text"
	case RubyKanji:
		return "ruby_kanji"
	case RubyFurigana:
		return "ruby_furigana"
	case Backlog:
		return "backlog"
	case File:
		return "file"
	case Other:
		return "other"
	default:
		return "other"
	}
}

func parseTextKind(value string) TextKind {
	switch value {
	case "name":
		return Name
	case "text":
		return Text
	case "ruby_kanji":
		return RubyKanji
	case "ruby_furigana":
		return RubyFurigana
	case "backlog":
		return Backlog
	case "file":
		return File
	default:
		return Other
	}
}

func kindRank(kind TextKind) int {
	switch kind {
	case Name:
		return 7
	case Text:
		return 6
	case RubyKanji:
		return 5
	case RubyFurigana:
		return 4
	case Backlog:
		return 3
	case File:
		return 2
	default:
		return 1
	}
}

func containerToString(kind ScriptContainerKind) string {
	if kind == RawCompiled {
		return "compiled"
	}
	return "dsc"
}

func parseContainer(value string) ScriptContainerKind {
	if value == "compiled" {
		return RawCompiled
	}
	return DscCompressed
}

func getEncoding(codepage uint32) (encoding.Encoding, error) {
	switch codepage {
	case 932:
		return japanese.ShiftJIS, nil
	default:
		return nil, fmt.Errorf("unsupported codepage %d", codepage)
	}
}

// DecodeScriptBytes decodes script bytes using the given codepage.
// Unrepresentable bytes are preserved as "&#XXXX" hexadecimal escapes.
func DecodeScriptBytes(b []byte, codepage uint32) (string, error) {
	enc, err := getEncoding(codepage)
	if err != nil {
		return "", err
	}
	dec := enc.NewDecoder()

	var out strings.Builder
	for i := 0; i < len(b); {
		n := 1
		if isDBCSLeadByte(b[i]) {
			if i+1 >= len(b) || !isDBCSTrailByte(b[i+1]) {
				out.WriteString(fmt.Sprintf("&#%04X", b[i]))
				i++
				continue
			}
			n = 2
		} else if !isSingleByteValid(b[i]) {
			out.WriteString(fmt.Sprintf("&#%04X", b[i]))
			i++
			continue
		}

		chunk := b[i : i+n]
		dst, _, err := transform.Bytes(dec, chunk)
		if err == nil && len(dst) > 0 {
			out.WriteString(string(dst))
		} else {
			if n == 2 {
				out.WriteString(fmt.Sprintf("&#%04X", uint16(b[i])<<8|uint16(b[i+1])))
			} else {
				out.WriteString(fmt.Sprintf("&#%04X", b[i]))
			}
		}
		i += n
	}
	return out.String(), nil
}

// EncodeScriptText encodes project text back to script bytes.
// "&#XXXX" escapes are restored as raw bytes; other text is encoded with
// the selected codepage. Unencodable characters produce an error.
func EncodeScriptText(text string, codepage uint32) ([]byte, error) {
	enc, err := getEncoding(codepage)
	if err != nil {
		return nil, err
	}
	encoder := enc.NewEncoder()

	var out []byte
	var chunk strings.Builder
	flush := func() error {
		if chunk.Len() == 0 {
			return nil
		}
		dst, _, err := transform.Bytes(encoder, []byte(chunk.String()))
		if err != nil {
			return fmt.Errorf("cannot encode text in codepage %d: %w", codepage, err)
		}
		out = append(out, dst...)
		chunk.Reset()
		return nil
	}

	for i := 0; i < len(text); {
		if text[i] == '&' && i+5 < len(text) && text[i+1] == '#' &&
			isHexDigit(text[i+2]) && isHexDigit(text[i+3]) &&
			isHexDigit(text[i+4]) && isHexDigit(text[i+5]) {
			if err := flush(); err != nil {
				return nil, err
			}
			val, _ := parseHexUint16(text[i+2 : i+6])
			if val > 0xFF {
				out = append(out, byte(val>>8))
			}
			out = append(out, byte(val))
			i += 6
			continue
		}
		chunk.WriteByte(text[i])
		i++
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

func isDBCSLeadByte(b byte) bool {
	// CP932 (ShiftJIS) lead byte ranges: 0x81-0x9F and 0xE0-0xFC.
	return (b >= 0x81 && b <= 0x9F) || (b >= 0xE0 && b <= 0xFC)
}

func isDBCSTrailByte(b byte) bool {
	// CP932 (ShiftJIS) trail byte ranges: 0x40-0x7E and 0x80-0xFC.
	return (b >= 0x40 && b <= 0x7E) || (b >= 0x80 && b <= 0xFC)
}

func isSingleByteValid(b byte) bool {
	// ASCII, control characters up to 0x1F, and half-width katakana 0xA1-0xDF.
	return b <= 0x7F || (b >= 0xA1 && b <= 0xDF)
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexValue(b byte) uint16 {
	switch {
	case b >= '0' && b <= '9':
		return uint16(b - '0')
	case b >= 'a' && b <= 'f':
		return uint16(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return uint16(b-'A') + 10
	}
	return 0
}

func parseHexUint16(s string) (uint16, error) {
	if len(s) != 4 {
		return 0, fmt.Errorf("invalid hex escape length")
	}
	var v uint16
	for i := 0; i < 4; i++ {
		if !isHexDigit(s[i]) {
			return 0, fmt.Errorf("invalid hex digit")
		}
		v = (v << 4) | hexValue(s[i])
	}
	return v, nil
}

// EscapeProjectText escapes characters that have special meaning in project files.
func EscapeProjectText(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			out.WriteString("\\\\")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			out.WriteByte(value[i])
		}
	}
	return out.String()
}

// UnescapeProjectText reverses EscapeProjectText.
func UnescapeProjectText(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			next := value[i+1]
			switch next {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '\\':
				out.WriteByte('\\')
			default:
				out.WriteByte(next)
			}
			i++
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}
