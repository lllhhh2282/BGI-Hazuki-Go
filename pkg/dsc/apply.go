package dsc

import (
	"fmt"
	"os"
)

// ApplyTextProject applies a translation project to a script file and writes
// the result. The script file is read directly from disk; any BSE wrapper must
// be stripped by the caller beforehand if necessary.
func ApplyTextProject(projectPath, scriptPath, outputPath string, fallbackEncodeCP uint32) error {
	project, err := LoadTextProject(projectPath)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	layout, err := LoadScriptLayout(scriptPath)
	if err != nil {
		return fmt.Errorf("load script: %w", err)
	}

	encodeCP := project.EncodeCodepage
	if fallbackEncodeCP != 0 {
		encodeCP = fallbackEncodeCP
	}

	return ApplyTextProjectToLayout(project, layout, outputPath, encodeCP)
}

// ApplyTextProjectToLayout applies a loaded translation project to an already
// parsed script layout and writes the result. This is useful when the script
// bytes have been preprocessed (for example BSE-decrypted) before parsing.
func ApplyTextProjectToLayout(project *TextProject, layout *ScriptLayout, outputPath string, encodeCP uint32) error {
	compiled, err := RebuildCompiledScript(layout, project, encodeCP)
	if err != nil {
		return fmt.Errorf("rebuild script: %w", err)
	}

	if layout.ContainerKind == DscCompressed {
		compiled, err = EncodeCompiledToDsc(compiled)
		if err != nil {
			return fmt.Errorf("recompress DSC: %w", err)
		}
	}

	return os.WriteFile(outputPath, compiled, 0o644)
}

// RebuildCompiledScript updates the text pool and code references using project entries.
func RebuildCompiledScript(layout *ScriptLayout, project *TextProject, encodeCP uint32) ([]byte, error) {
	if layout == nil || project == nil {
		return nil, fmt.Errorf("nil layout or project")
	}

	replacementBytes := make(map[uint32][]byte, len(project.Entries))
	for _, entry := range project.Entries {
		text := entry.TranslationText
		if text == "" {
			text = entry.OriginalText
		}
		b, err := EncodeScriptText(text, encodeCP)
		if err != nil {
			return nil, fmt.Errorf("encode entry %04d: %w", entry.Index, err)
		}
		replacementBytes[entry.TextOffset] = b
	}

	newTextBytes := make([]byte, 0)
	newOffsets := make(map[uint32]uint32, len(layout.TextSlots))
	for _, slot := range layout.TextSlots {
		newOffsets[slot.Offset] = uint32(len(newTextBytes))
		if b, ok := replacementBytes[slot.Offset]; ok {
			newTextBytes = append(newTextBytes, b...)
		} else {
			newTextBytes = append(newTextBytes, slot.Bytes...)
		}
		newTextBytes = append(newTextBytes, 0)
	}

	newCodeBytes := append([]byte(nil), layout.CodeBytes...)
	codeSize := uint32(len(layout.CodeBytes))
	for pos := uint32(4); pos+4 <= codeSize; pos += 4 {
		value := readU32LE(newCodeBytes, int(pos))
		if value < codeSize {
			continue
		}
		oldTextOffset := value - codeSize
		if newOffset, ok := newOffsets[oldTextOffset]; ok {
			writeU32LE(newCodeBytes, int(pos), codeSize+newOffset)
		}
	}

	compiled := make([]byte, 0, len(layout.HeaderBytes)+len(newCodeBytes)+len(newTextBytes))
	compiled = append(compiled, layout.HeaderBytes...)
	compiled = append(compiled, newCodeBytes...)
	compiled = append(compiled, newTextBytes...)
	return compiled, nil
}
