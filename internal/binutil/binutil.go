// Package binutil provides little-endian binary I/O helpers and bit/stream utilities.
package binutil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrUnexpectedEOF = errors.New("unexpected EOF")
	ErrVarIntTooLong = errors.New("varint too long")
)

// ReadU16LE reads a little-endian uint16 from b at offset.
func ReadU16LE(b []byte, offset int) uint16 {
	return binary.LittleEndian.Uint16(b[offset:])
}

// ReadU32LE reads a little-endian uint32 from b at offset.
func ReadU32LE(b []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(b[offset:])
}

// ReadU64LE reads a little-endian uint64 from b at offset.
func ReadU64LE(b []byte, offset int) uint64 {
	return binary.LittleEndian.Uint64(b[offset:])
}

// WriteU16LE writes a little-endian uint16 to b at offset.
func WriteU16LE(b []byte, offset int, v uint16) {
	binary.LittleEndian.PutUint16(b[offset:], v)
}

// WriteU32LE writes a little-endian uint32 to b at offset.
func WriteU32LE(b []byte, offset int, v uint32) {
	binary.LittleEndian.PutUint32(b[offset:], v)
}

// WriteU64LE writes a little-endian uint64 to b at offset.
func WriteU64LE(b []byte, offset int, v uint64) {
	binary.LittleEndian.PutUint64(b[offset:], v)
}

// BitReader reads bits from a byte slice, LSB first.
type BitReader struct {
	data   []byte
	bytePos int
	bitPos  uint8 // 0..7, current bit within data[bytePos]
}

// NewBitReader creates a new BitReader.
func NewBitReader(data []byte) *BitReader {
	return &BitReader{data: data}
}

// ReadBit reads a single bit. Returns 0 or 1.
func (r *BitReader) ReadBit() (int, error) {
	if r.bytePos >= len(r.data) {
		return 0, ErrUnexpectedEOF
	}
	bit := (r.data[r.bytePos] >> r.bitPos) & 1
	r.bitPos++
	if r.bitPos == 8 {
		r.bitPos = 0
		r.bytePos++
	}
	return int(bit), nil
}

// ReadBits reads n bits as a uint32, LSB first.
func (r *BitReader) ReadBits(n int) (uint32, error) {
	if n < 0 || n > 32 {
		return 0, errors.New("invalid bit count")
	}
	var v uint32
	for i := 0; i < n; i++ {
		bit, err := r.ReadBit()
		if err != nil {
			return 0, err
		}
		if bit != 0 {
			v |= 1 << i
		}
	}
	return v, nil
}

// ReadBool reads a single bit as a bool.
func (r *BitReader) ReadBool() (bool, error) {
	bit, err := r.ReadBit()
	return bit != 0, err
}

// ByteAligned reports whether the reader is currently at a byte boundary.
func (r *BitReader) ByteAligned() bool {
	return r.bitPos == 0
}

// ByteOffset returns the current byte offset (rounded down).
func (r *BitReader) ByteOffset() int {
	return r.bytePos
}

// BitWriter writes bits to a bytes.Buffer, LSB first.
type BitWriter struct {
	buf    *bytes.Buffer
	cur    byte
	bitPos uint8
}

// NewBitWriter creates a new BitWriter.
func NewBitWriter(buf *bytes.Buffer) *BitWriter {
	return &BitWriter{buf: buf}
}

// WriteBit writes a single bit.
func (w *BitWriter) WriteBit(bit int) {
	if bit != 0 {
		w.cur |= 1 << w.bitPos
	}
	w.bitPos++
	if w.bitPos == 8 {
		w.buf.WriteByte(w.cur)
		w.cur = 0
		w.bitPos = 0
	}
}

// WriteBits writes the low n bits of v.
func (w *BitWriter) WriteBits(v uint32, n int) {
	for i := 0; i < n; i++ {
		if (v>>i)&1 != 0 {
			w.WriteBit(1)
		} else {
			w.WriteBit(0)
		}
	}
}

// Flush pads remaining bits with zeros and writes the final byte.
func (w *BitWriter) Flush() {
	if w.bitPos != 0 {
		w.buf.WriteByte(w.cur)
		w.cur = 0
		w.bitPos = 0
	}
}

// ReadVarInt reads a 7-bit LSB-first variable-length unsigned integer.
func ReadVarInt(r io.ByteReader) (uint32, error) {
	var v uint32
	var shift uint32
	for {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, ErrUnexpectedEOF
			}
			return 0, err
		}
		v |= uint32(b&0x7F) << shift
		if (b & 0x80) == 0 {
			return v, nil
		}
		shift += 7
		if shift >= 32 {
			return 0, ErrVarIntTooLong
		}
	}
}

// WriteVarInt writes a 7-bit LSB-first variable-length unsigned integer.
func WriteVarInt(w *bytes.Buffer, v uint32) {
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		w.WriteByte(b)
		if v == 0 {
			break
		}
	}
}
