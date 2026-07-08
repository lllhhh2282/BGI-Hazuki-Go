package dsc

import (
	"bytes"
	"fmt"
)

const (
	dscSymbolCount  = 512
	dscLiteralCount = 256
	dscDistanceBits = 12
	dscMinDistance  = 2
	dscMinMatchLen  = 3
	dscMaxMatchLen  = 257
)

// dscNode is a node in the DSC Huffman tree.
type dscNode struct {
	hasChildren bool
	lookBehind  bool
	value       uint8
	children    [2]uint32
}

// msbBitReader reads bits in most-significant-bit-first order.
type msbBitReader struct {
	data    []byte
	bytePos int
	bitPos  uint8 // 0 = MSB
}

func newMsbBitReader(data []byte) *msbBitReader {
	return &msbBitReader{data: data}
}

func (r *msbBitReader) readBit() (int, error) {
	if r.bytePos >= len(r.data) {
		return 0, fmt.Errorf("unexpected end of DSC bitstream")
	}
	bit := (r.data[r.bytePos] >> (7 - r.bitPos)) & 1
	r.bitPos++
	if r.bitPos == 8 {
		r.bitPos = 0
		r.bytePos++
	}
	return int(bit), nil
}

func (r *msbBitReader) readBits(n int) (uint32, error) {
	if n < 0 || n > 32 {
		return 0, fmt.Errorf("invalid bit count %d", n)
	}
	var v uint32
	for i := 0; i < n; i++ {
		bit, err := r.readBit()
		if err != nil {
			return 0, err
		}
		v = (v << 1) | uint32(bit)
	}
	return v, nil
}

// nextKeyByte advances the DSC LCG and returns one keystream byte.
func nextKeyByte(key *uint32) uint8 {
	v0 := uint32(20021) * (*key & 0xFFFF)
	v1 := *key >> 16
	v1 = v1*20021 + *key*346
	v1 = (v1 + (v0 >> 16)) & 0xFFFF
	*key = (v1 << 16) + (v0 & 0xFFFF) + 1
	return uint8(v1 & 0xFF)
}

// buildDscTree builds the Huffman tree used by DSC containers from code lengths.
func buildDscTree(lengths []uint8) ([1024]dscNode, error) {
	if len(lengths) != dscSymbolCount {
		return [1024]dscNode{}, fmt.Errorf("invalid DSC code length table size %d", len(lengths))
	}

	var nodes [1024]dscNode

	type entry struct {
		length uint32
		symbol uint32
	}
	entries := make([]entry, 0, dscSymbolCount)
	for n := 0; n < dscSymbolCount; n++ {
		if lengths[n] != 0 {
			entries = append(entries, entry{length: uint32(lengths[n]), symbol: uint32(n)})
		}
	}
	// Sort by length first, then by symbol.
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].length < entries[i].length ||
				(entries[j].length == entries[i].length && entries[j].symbol < entries[i].symbol) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	var arr1 [1024]uint32
	var unk0 uint32 = 0x200
	var unk1 uint32 = 1
	var nodeIndex uint32 = 1
	var nodePtr uint32 = 0
	var entryPos int = 0

	for level := uint32(0); entryPos < len(entries); level++ {
		arr1Ptr := unk0
		arr1OldPtr := arr1Ptr
		groupCount := uint32(0)

		for {
			var current uint32
			if entryPos < len(entries) {
				current = entries[entryPos].length<<16 + entries[entryPos].symbol
			}
			if level != (current >> 16) {
				break
			}
			node := &nodes[arr1[nodePtr]]
			node.hasChildren = false
			node.lookBehind = (current & 0x100) != 0
			node.value = uint8(current & 0xFF)
			entryPos++
			nodePtr++
			groupCount++
		}

		nextWidth := 2 * (unk1 - groupCount)
		if groupCount < unk1 {
			unk1 -= groupCount
			for i := uint32(0); i < unk1; i++ {
				node := &nodes[arr1[nodePtr]]
				node.hasChildren = true
				for branch := 0; branch < 2; branch++ {
					arr1[arr1Ptr] = nodeIndex
					node.children[branch] = nodeIndex
					arr1Ptr++
					nodeIndex++
				}
				nodePtr++
			}
		}
		unk1 = nextWidth
		nodePtr = arr1OldPtr
		unk0 ^= 0x200
	}

	return nodes, nil
}

// DecodeDscToCompiled decompresses a DSC container into raw compiled script bytes.
func DecodeDscToCompiled(input []byte) ([]byte, error) {
	if len(input) < len(dscMagic)+16 || !bytes.Equal(input[:len(dscMagic)], dscMagic) {
		return nil, errNotDscScript
	}

	key := readU32LE(input, 0x10)
	outputSize := readU32LE(input, 0x14)
	// 0x18..0x1F are reserved and ignored.

	if len(input) < 0x220 {
		return nil, fmt.Errorf("DSC input too short for code length table")
	}
	lengths := make([]uint8, dscSymbolCount)
	for i := 0; i < dscSymbolCount; i++ {
		lengths[i] = input[0x20+i] - nextKeyByte(&key)
	}

	nodes, err := buildDscTree(lengths)
	if err != nil {
		return nil, err
	}

	bits := newMsbBitReader(input[0x220:])
	output := make([]byte, 0, outputSize)

	for uint32(len(output)) < outputSize {
		nodeIndex := uint32(0)
		for nodes[nodeIndex].hasChildren {
			bit, err := bits.readBit()
			if err != nil {
				return nil, fmt.Errorf("DSC huffman decode truncated: %w", err)
			}
			nodeIndex = nodes[nodeIndex].children[bit]
		}

		if nodes[nodeIndex].lookBehind {
			offset, err := bits.readBits(dscDistanceBits)
			if err != nil {
				return nil, fmt.Errorf("DSC distance bits truncated: %w", err)
			}
			repetitions := uint32(nodes[nodeIndex].value) + dscMinDistance
			if uint32(len(output)) < offset+dscMinDistance {
				return nil, fmt.Errorf("DSC look-behind offset is invalid")
			}
			lookBehind := uint32(len(output)) - offset - dscMinDistance
			for repetitions > 0 && uint32(len(output)) < outputSize {
				output = append(output, output[lookBehind])
				lookBehind++
				repetitions--
			}
		} else {
			output = append(output, nodes[nodeIndex].value)
		}
	}

	if uint32(len(output)) != outputSize {
		return nil, fmt.Errorf("DSC decoded size mismatch")
	}
	return output, nil
}

func readU32LE(b []byte, offset int) uint32 {
	return uint32(b[offset]) | uint32(b[offset+1])<<8 | uint32(b[offset+2])<<16 | uint32(b[offset+3])<<24
}

func writeU32LE(b []byte, offset int, v uint32) {
	b[offset] = byte(v)
	b[offset+1] = byte(v >> 8)
	b[offset+2] = byte(v >> 16)
	b[offset+3] = byte(v >> 24)
}
