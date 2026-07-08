// Package cbg implements encoding and decoding of BURIKO CompressedBG (CBG) images.
package cbg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

const (
	magicSize  = 16
	headerSize = 0x30 // 48 bytes
)

var magic = []byte{
	0x43, 0x6F, 0x6D, 0x70, 0x72, 0x65, 0x73, 0x73,
	0x65, 0x64, 0x42, 0x47, 0x5F, 0x5F, 0x5F, 0x00,
}

// RasterImage holds a decoded CBG image in BGRA32 format.
type RasterImage struct {
	Width    uint32
	Height   uint32
	HasAlpha bool
	Pixels   []uint8 // BGRA32, length = Width*Height*4
}

// cbgHeader is the 48-byte CBG file header.
type cbgHeader struct {
	Width              uint16
	Height             uint16
	BitsPerPixel       uint32
	Reserved           [8]byte
	IntermediateLength uint32 // v1: RLE stream length; v2: 0
	Key                uint32
	EncLength          uint32
	ChecksumSum        uint8
	ChecksumXor        uint8
	Version            uint16
}

var (
	// ErrNotCbg is returned when a file does not start with the CBG magic.
	ErrNotCbg = errors.New("not a CompressedBG image")
)

// IsCbgFile reports whether path points to a CBG file.
func IsCbgFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return IsCbgBytes(data), nil
}

// IsCbgBytes reports whether data starts with the CBG magic.
func IsCbgBytes(data []byte) bool {
	return len(data) >= magicSize && bytes.Equal(data[:magicSize], magic)
}

func readHeader(data []byte) (*cbgHeader, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("cbg: file too small for header (%d bytes, need %d)", len(data), headerSize)
	}
	if !bytes.Equal(data[:magicSize], magic) {
		return nil, ErrNotCbg
	}

	h := &cbgHeader{}
	h.Width = binary.LittleEndian.Uint16(data[0x10:])
	h.Height = binary.LittleEndian.Uint16(data[0x12:])
	h.BitsPerPixel = binary.LittleEndian.Uint32(data[0x14:])
	copy(h.Reserved[:], data[0x18:0x20])
	h.IntermediateLength = binary.LittleEndian.Uint32(data[0x20:])
	h.Key = binary.LittleEndian.Uint32(data[0x24:])
	h.EncLength = binary.LittleEndian.Uint32(data[0x28:])
	h.ChecksumSum = data[0x2C]
	h.ChecksumXor = data[0x2D]
	h.Version = binary.LittleEndian.Uint16(data[0x2E:])
	return h, nil
}

func writeHeader(buf *bytes.Buffer, h *cbgHeader) {
	buf.Write(magic)
	writeU16(buf, h.Width)
	writeU16(buf, h.Height)
	writeU32(buf, h.BitsPerPixel)
	buf.Write(h.Reserved[:])
	writeU32(buf, h.IntermediateLength)
	writeU32(buf, h.Key)
	writeU32(buf, h.EncLength)
	buf.WriteByte(h.ChecksumSum)
	buf.WriteByte(h.ChecksumXor)
	writeU16(buf, h.Version)
}

func writeU16(buf *bytes.Buffer, v uint16) {
	buf.WriteByte(byte(v & 0xFF))
	buf.WriteByte(byte(v >> 8))
}

func writeU32(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v & 0xFF))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 24))
}

// nextKeyByte advances the LCG and returns one keystream byte.
func nextKeyByte(key *uint32) uint8 {
	v0 := uint32(20021) * (*key & 0xFFFF)
	v1 := *key >> 16
	v1 = v1*20021 + *key*346
	v1 = (v1 + (v0 >> 16)) & 0xFFFF
	*key = (v1 << 16) + (v0 & 0xFFFF) + 1
	return uint8(v1 & 0xFF)
}

// decrypt decrypts data in place using the LCG keystream and returns the checksums.
func decrypt(data []byte, key uint32) (sum, xor uint8, err error) {
	k := key
	for i := range data {
		data[i] -= nextKeyByte(&k)
		sum += data[i]
		xor ^= data[i]
	}
	return sum, xor, nil
}

// encrypt encrypts data in place using the LCG keystream and computes checksums over the plaintext.
func encrypt(data []byte, key uint32) (sum, xor uint8) {
	k := key
	for i := range data {
		sum += data[i]
		xor ^= data[i]
		data[i] += nextKeyByte(&k)
	}
	return sum, xor
}
