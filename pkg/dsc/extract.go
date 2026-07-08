package dsc

import (
	"fmt"
	"sort"
)

// ExtractedEntryInfo accumulates classification results while scanning code.
type extractedEntryInfo struct {
	kind        TextKind
	comment     string
	codeOffsets []uint32
}

func toTextSlotMap(slots []TextSlot) map[uint32]TextSlot {
	m := make(map[uint32]TextSlot, len(slots))
	for _, slot := range slots {
		m[slot.Offset] = slot
	}
	return m
}

func checkFunction(code []byte, pos uint32, functionID uint32, relativeOffset int32) bool {
	if relativeOffset < 0 && uint32(-relativeOffset) > pos {
		return false
	}
	target := int64(pos) + int64(relativeOffset)
	if target < 0 || target+4 > int64(len(code)) {
		return false
	}
	return readU32LE(code, int(target)) == functionID
}

// ClassifyStringReference determines the role of a string pointer at code offset pos.
func ClassifyStringReference(code []byte, pos uint32, config ScriptConfig, textSlots map[uint32]TextSlot, decodeCodepage uint32) (TextKind, string, error) {
	opcode := readU32LE(code, int(pos)-4)
	if opcode == config.FileType {
		return File, "FILE", nil
	}
	if opcode != config.StringType {
		return Other, "OTHER", nil
	}

	if checkFunction(code, pos, config.TextFunction, config.NamePos) {
		return Name, "NAME", nil
	}
	if checkFunction(code, pos, config.TextFunction, config.TextPos) {
		comment := "TEXT"
		delta := config.TextPos - config.NamePos
		namePointerPos := int64(pos) + int64(delta)
		if namePointerPos >= 0 && namePointerPos+4 <= int64(len(code)) {
			nameDword := readU32LE(code, int(namePointerPos))
			if nameDword != 0 {
				nameOffset := nameDword - uint32(len(code))
				if slot, ok := textSlots[nameOffset]; ok {
					nameText, err := DecodeScriptBytes(slot.Bytes, decodeCodepage)
					if err == nil {
						comment += " 【" + nameText + "】"
					}
				}
			}
		}
		return Text, comment, nil
	}
	if checkFunction(code, pos, config.RubyFunction, config.RubyKanjiPos) {
		return RubyKanji, "TEXT RUBY KANJI", nil
	}
	if checkFunction(code, pos, config.RubyFunction, config.RubyFuriganaPos) {
		return RubyFurigana, "TEXT RUBY FURIGANA", nil
	}
	if checkFunction(code, pos, config.BacklogFunction, config.BacklogPos) {
		return Backlog, "TEXT BACKLOG", nil
	}
	return Other, "OTHER", nil
}

func extractEntries(layout *ScriptLayout, decodeCodepage uint32) ([]TextEntry, error) {
	textSlotMap := toTextSlotMap(layout.TextSlots)
	infoByOffset := make(map[uint32]*extractedEntryInfo)

	for pos := uint32(4); pos+4 <= uint32(len(layout.CodeBytes)); pos += 4 {
		value := readU32LE(layout.CodeBytes, int(pos))
		if value < uint32(len(layout.CodeBytes)) {
			continue
		}
		textOffset := value - uint32(len(layout.CodeBytes))
		if _, ok := textSlotMap[textOffset]; !ok {
			continue
		}

		kind, comment, err := ClassifyStringReference(layout.CodeBytes, pos, layout.Config, textSlotMap, decodeCodepage)
		if err != nil {
			return nil, err
		}

		info, exists := infoByOffset[textOffset]
		if !exists {
			info = &extractedEntryInfo{}
			infoByOffset[textOffset] = info
		}
		if info.comment == "" || kindRank(kind) > kindRank(info.kind) {
			info.kind = kind
			info.comment = comment
		}
		info.codeOffsets = append(info.codeOffsets, pos)
	}

	offsets := make([]uint32, 0, len(infoByOffset))
	for offset := range infoByOffset {
		offsets = append(offsets, offset)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })

	entries := make([]TextEntry, 0, len(offsets))
	index := uint32(1)
	for _, offset := range offsets {
		info := infoByOffset[offset]
		slot := textSlotMap[offset]
		originalText, err := DecodeScriptBytes(slot.Bytes, decodeCodepage)
		if err != nil {
			return nil, fmt.Errorf("decode text at offset %08X: %w", offset, err)
		}
		entries = append(entries, TextEntry{
			Index:         index,
			TextOffset:    offset,
			Kind:          info.kind,
			Comment:       info.comment,
			CodeOffsets:   append([]uint32(nil), info.codeOffsets...),
			OriginalBytes: append([]byte(nil), slot.Bytes...),
			OriginalText:  originalText,
		})
		index++
	}
	return entries, nil
}

// ExtractTextProject extracts all translatable strings from a script file.
func ExtractTextProject(scriptPath string, decodeCP, encodeCP uint32) (*TextProject, error) {
	layout, err := LoadScriptLayout(scriptPath)
	if err != nil {
		return nil, err
	}
	return ExtractTextProjectFromLayout(layout, decodeCP, encodeCP)
}

// ExtractTextProjectFromLayout extracts translatable strings from an already
// parsed compiled-script layout.
func ExtractTextProjectFromLayout(layout *ScriptLayout, decodeCP, encodeCP uint32) (*TextProject, error) {
	entries, err := extractEntries(layout, decodeCP)
	if err != nil {
		return nil, err
	}
	return &TextProject{
		ContainerKind:  layout.ContainerKind,
		DecodeCodepage: decodeCP,
		EncodeCodepage: encodeCP,
		Entries:        entries,
	}, nil
}
