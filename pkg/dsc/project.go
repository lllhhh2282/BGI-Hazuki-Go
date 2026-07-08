package dsc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SaveTextProject writes a TextProject to a UTF-8-BOM .hazuki.txt file.
func SaveTextProject(project *TextProject, scriptPath, outputPath string) error {
	content, err := makeProjectText(project, scriptPath)
	if err != nil {
		return err
	}
	bytes := append([]byte{0xEF, 0xBB, 0xBF}, []byte(content)...)
	return os.WriteFile(outputPath, bytes, 0o644)
}

// LoadTextProject reads a .hazuki.txt project file.
func LoadTextProject(path string) (*TextProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	return parseProjectText(string(data))
}

func makeProjectText(project *TextProject, scriptPath string) (string, error) {
	if project == nil {
		return "", fmt.Errorf("nil project")
	}
	var b strings.Builder
	b.WriteString(projectMagic)
	b.WriteByte('\n')
	b.WriteString("# script_name=")
	b.WriteString(filepath.Base(scriptPath))
	b.WriteByte('\n')
	b.WriteString("# container=")
	b.WriteString(containerToString(project.ContainerKind))
	b.WriteByte('\n')
	b.WriteString("# decode_cp=")
	b.WriteString(strconv.FormatUint(uint64(project.DecodeCodepage), 10))
	b.WriteByte('\n')
	b.WriteString("# encode_cp=")
	b.WriteString(strconv.FormatUint(uint64(project.EncodeCodepage), 10))
	b.WriteByte('\n')
	b.WriteByte('\n')

	for _, entry := range project.Entries {
		b.WriteString("[ENTRY]\n")
		b.WriteString(fmt.Sprintf("id=%04d\n", entry.Index))
		b.WriteString("kind=")
		b.WriteString(ToString(entry.Kind))
		b.WriteByte('\n')
		b.WriteString(fmt.Sprintf("text_offset=%08X\n", entry.TextOffset))
		b.WriteString("code_offsets=")
		b.WriteString(joinHexOffsets(entry.CodeOffsets))
		b.WriteByte('\n')
		b.WriteString("comment=")
		b.WriteString(EscapeProjectText(entry.Comment))
		b.WriteByte('\n')
		b.WriteString("src=")
		b.WriteString(EscapeProjectText(entry.OriginalText))
		b.WriteByte('\n')
		b.WriteString("dst=")
		b.WriteString(EscapeProjectText(entry.TranslationText))
		b.WriteByte('\n')
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func joinHexOffsets(offsets []uint32) string {
	var b strings.Builder
	for i, off := range offsets {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%08X", off))
	}
	return b.String()
}

func parseHexOffsets(value string) []uint32 {
	var offsets []uint32
	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		v, err := strconv.ParseUint(token, 16, 32)
		if err != nil {
			continue
		}
		offsets = append(offsets, uint32(v))
	}
	return offsets
}

func parseProjectText(content string) (*TextProject, error) {
	lines := splitLines(content)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != projectMagic {
		return nil, fmt.Errorf("not a BGI_Hazuki text project")
	}

	project := &TextProject{
		DecodeCodepage: defaultCodepage,
		EncodeCodepage: defaultCodepage,
	}
	var current *TextEntry
	pushEntry := func() {
		if current != nil {
			project.Entries = append(project.Entries, *current)
			current = nil
		}
	}

	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			pushEntry()
			continue
		}
		if line[0] == '#' {
			sep := strings.IndexByte(line, '=')
			if sep == -1 {
				continue
			}
			key := strings.TrimSpace(line[1:sep])
			value := strings.TrimSpace(line[sep+1:])
			switch key {
			case "container":
				project.ContainerKind = parseContainer(value)
			case "decode_cp":
				if v, err := strconv.ParseUint(value, 10, 32); err == nil {
					project.DecodeCodepage = uint32(v)
				}
			case "encode_cp":
				if v, err := strconv.ParseUint(value, 10, 32); err == nil {
					project.EncodeCodepage = uint32(v)
				}
			}
			continue
		}
		if line == "[ENTRY]" {
			pushEntry()
			current = &TextEntry{}
			continue
		}
		if current == nil {
			continue
		}
		sep := strings.IndexByte(line, '=')
		if sep == -1 {
			continue
		}
		key := strings.TrimSpace(line[:sep])
		value := line[sep+1:]
		switch key {
		case "id":
			if v, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32); err == nil {
				current.Index = uint32(v)
			}
		case "kind":
			current.Kind = parseTextKind(strings.TrimSpace(value))
		case "text_offset":
			if v, err := strconv.ParseUint(strings.TrimSpace(value), 16, 32); err == nil {
				current.TextOffset = uint32(v)
			}
		case "code_offsets":
			current.CodeOffsets = parseHexOffsets(value)
		case "comment":
			current.Comment = UnescapeProjectText(value)
		case "src":
			current.OriginalText = UnescapeProjectText(value)
		case "dst":
			current.TranslationText = UnescapeProjectText(value)
		}
	}
	pushEntry()
	return project, nil
}

func splitLines(value string) []string {
	var lines []string
	var current strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\r' {
			continue
		}
		if value[i] == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(value[i])
	}
	lines = append(lines, current.String())
	return lines
}
