package dsc

import (
	"bytes"
	"fmt"
	"os"
)

// ScriptConfig holds version-specific constants used to interpret a script.
type ScriptConfig struct {
	HeaderSize          uint32
	AdditionalHeaderPos *uint32
	StringType          uint32
	FileType            uint32
	TextFunction        uint32
	BacklogFunction     uint32
	RubyFunction        uint32
	NamePos             int32
	TextPos             int32
	RubyKanjiPos        int32
	RubyFuriganaPos     int32
	BacklogPos          int32
}

// TextSlot describes one null-terminated string in the text pool.
type TextSlot struct {
	Offset uint32
	Bytes  []byte
}

// ScriptLayout holds the parsed sections of a compiled script.
type ScriptLayout struct {
	ContainerKind ScriptContainerKind
	Config        ScriptConfig
	CompiledData  []byte
	HeaderBytes   []byte
	CodeBytes     []byte
	TextSlots     []TextSlot
}

func getVer000Config() ScriptConfig {
	return ScriptConfig{
		HeaderSize:          0x0,
		AdditionalHeaderPos: nil,
		StringType:          0x3,
		FileType:            0x7F,
		TextFunction:        0x140,
		BacklogFunction:     0x143,
		RubyFunction:        0x14B,
		NamePos:             0x24,
		TextPos:             0x2C,
		RubyKanjiPos:        0x14,
		RubyFuriganaPos:     0x0C,
		BacklogPos:          0x0C,
	}
}

func getVer100Config() ScriptConfig {
	pos := uint32(0x1C)
	return ScriptConfig{
		HeaderSize:          0x1C,
		AdditionalHeaderPos: &pos,
		StringType:          0x3,
		FileType:            0x7F,
		TextFunction:        0x140,
		BacklogFunction:     0x143,
		RubyFunction:        0x14B,
		NamePos:             0x0C,
		TextPos:             0x04,
		RubyKanjiPos:        0x04,
		RubyFuriganaPos:     0x0C,
		BacklogPos:          0x0C,
	}
}

func getScriptConfig(compiled []byte) ScriptConfig {
	if len(compiled) >= len(compiledMagic) && string(compiled[:len(compiledMagic)]) == string(compiledMagic) {
		return getVer100Config()
	}
	return getVer000Config()
}

// FindTextBoundary locates the start of the text pool by scanning for the
// last "1B 00 00 00" marker. If none is found, it returns len(compiled).
func FindTextBoundary(compiled []byte) uint32 {
	var found int = -1
	for pos := 0; pos+4 <= len(compiled); pos++ {
		if compiled[pos] == 0x1B && compiled[pos+1] == 0x00 && compiled[pos+2] == 0x00 && compiled[pos+3] == 0x00 {
			found = pos
		}
	}
	if found == -1 {
		return uint32(len(compiled))
	}
	return uint32(found + 4)
}

// ParseTextSlots splits the text pool bytes into null-terminated slots.
func ParseTextSlots(textBytes []byte) []TextSlot {
	var slots []TextSlot
	if len(textBytes) == 0 || (len(textBytes) == 1 && textBytes[0] == 0) {
		return slots
	}

	position := 0
	for position < len(textBytes) {
		end := position
		for end < len(textBytes) && textBytes[end] != 0 {
			end++
		}
		slots = append(slots, TextSlot{
			Offset: uint32(position),
			Bytes:  append([]byte(nil), textBytes[position:end]...),
		})
		if end == len(textBytes) {
			break
		}
		position = end + 1
		if position == len(textBytes) {
			break
		}
	}
	return slots
}

// LoadScriptLayout reads a script file and splits it into header, code, and text sections.
func LoadScriptLayout(path string) (*ScriptLayout, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(raw) >= len(dscMagic) && bytes.Equal(raw[:len(dscMagic)], dscMagic) {
		return LoadScriptLayoutFromContainer(raw, DscCompressed)
	}
	return LoadScriptLayoutFromContainer(raw, RawCompiled)
}

// LoadScriptLayoutFromContainer builds a ScriptLayout from raw on-disk bytes.
// The caller must indicate whether the bytes are a DSC container or a raw
// compiled script.
func LoadScriptLayoutFromContainer(raw []byte, kind ScriptContainerKind) (*ScriptLayout, error) {
	layout := &ScriptLayout{ContainerKind: kind}
	var err error
	if kind == DscCompressed {
		layout.CompiledData, err = DecodeDscToCompiled(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress DSC: %w", err)
		}
	} else {
		layout.CompiledData = raw
	}

	return finishScriptLayout(layout)
}

// LoadScriptLayoutFromCompiled builds a ScriptLayout from already-decompiled
// compiled script bytes.
func LoadScriptLayoutFromCompiled(compiled []byte) (*ScriptLayout, error) {
	return LoadScriptLayoutFromCompiledWithKind(compiled, DscCompressed)
}

// LoadScriptLayoutFromCompiledWithKind builds a ScriptLayout from decompiled
// bytes while preserving the original container kind.
func LoadScriptLayoutFromCompiledWithKind(compiled []byte, kind ScriptContainerKind) (*ScriptLayout, error) {
	layout := &ScriptLayout{
		ContainerKind: kind,
		CompiledData:  compiled,
	}
	return finishScriptLayout(layout)
}

func finishScriptLayout(layout *ScriptLayout) (*ScriptLayout, error) {
	layout.Config = getScriptConfig(layout.CompiledData)
	headerSize := layout.Config.HeaderSize
	if layout.Config.AdditionalHeaderPos != nil {
		headerSize += readU32LE(layout.CompiledData, int(*layout.Config.AdditionalHeaderPos))
	}

	textBoundary := FindTextBoundary(layout.CompiledData)
	if uint64(headerSize) > uint64(textBoundary) || uint64(textBoundary) > uint64(len(layout.CompiledData)) {
		return nil, fmt.Errorf("script section boundaries are invalid")
	}

	layout.HeaderBytes = append([]byte(nil), layout.CompiledData[:headerSize]...)
	layout.CodeBytes = append([]byte(nil), layout.CompiledData[headerSize:textBoundary]...)
	textBytes := layout.CompiledData[textBoundary:]
	layout.TextSlots = ParseTextSlots(textBytes)
	return layout, nil
}
