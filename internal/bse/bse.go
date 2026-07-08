// Package bse implements BGI "BSE 1.0" container decryption.
//
// The algorithm is an independent reimplementation based on the public
// behaviour of the BGI engine format, released as part of BGI-Hazuki-Go
// under the Unlicense (see LICENSE in the project root).
package bse

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
)

const (
	magicSize     = 7
	headerSize    = 16
	encryptedSize = 64
	minSize       = headerSize + encryptedSize
)

var magic = []byte("BSE 1.0")

var (
	ErrNotBse     = errors.New("not a BSE 1.0 container")
	ErrInvalidBse = errors.New("BSE container is malformed or checksum mismatch")
)

// IsBseContainer reports whether data starts with the BSE 1.0 signature.
func IsBseContainer(data []byte) bool {
	return len(data) >= minSize && string(data[:magicSize]) == string(magic)
}

// DecryptBse decrypts a BSE 1.0 container in memory and returns the inner
// payload with the 16-byte BSE header removed. The caller should treat the
// returned bytes as the underlying CBG, DSC, or other asset file.
func DecryptBse(data []byte) ([]byte, error) {
	if !IsBseContainer(data) {
		return nil, ErrNotBse
	}

	// Copy the encrypted region so the original slice is not modified on failure.
	inner := make([]byte, len(data)-headerSize)
	copy(inner, data[headerSize:])

	// Parse the BSE header fields (little-endian).
	// data[8:10] is a uint16 usually equal to 0x0100.
	_ = binary.LittleEndian.Uint16(data[8:10])
	sumCheck := data[10]
	xorCheck := data[11]
	seed := binary.LittleEndian.Uint32(data[12:16])

	decrypted := inner[:encryptedSize]
	flags := [encryptedSize]bool{}

	for counter := 0; counter < encryptedSize; counter++ {
		r := bseRand(&seed)
		i := r & 0x3F
		for flags[i] {
			i = (i + 1) & 0x3F
		}

		r = bseRand(&seed)
		s := r & 0x07
		target := i

		k := bseRand(&seed)
		r = bseRand(&seed)

		plain := (uint32(decrypted[target]) - r) & 0xFF
		if k&1 != 0 {
			decrypted[target] = bits.RotateLeft8(uint8(plain), int(s))
		} else {
			decrypted[target] = bits.RotateLeft8(uint8(plain), -int(s))
		}
		flags[i] = true
	}

	var sumData, xorData uint8
	for _, b := range decrypted {
		sumData += b
		xorData ^= b
	}

	if sumData != sumCheck || xorData != xorCheck {
		return nil, fmt.Errorf("%w (sum %02x/%02x, xor %02x/%02x)",
			ErrInvalidBse, sumData, sumCheck, xorData, xorCheck)
	}

	return inner, nil
}

// EncryptBse builds a BSE 1.0 container around the first 64 bytes of inner.
// The returned slice has the 16-byte BSE header followed by the (encrypted)
// inner payload. It is provided mainly for testing and symmetry; most users
// only need DecryptBse.
func EncryptBse(inner []byte, seed uint32) ([]byte, error) {
	if len(inner) < encryptedSize {
		return nil, fmt.Errorf("BSE payload must be at least %d bytes", encryptedSize)
	}

	out := make([]byte, headerSize+len(inner))
	copy(out, magic)
	// out[7] is left as the null terminator of the magic string.
	binary.LittleEndian.PutUint16(out[8:10], 0x0100)

	var sum, xor uint8
	for i := 0; i < encryptedSize; i++ {
		sum += inner[i]
		xor ^= inner[i]
	}
	out[10] = sum
	out[11] = xor
	binary.LittleEndian.PutUint32(out[12:16], seed)
	copy(out[headerSize:], inner)

	enc := out[headerSize : headerSize+encryptedSize]
	flags := [encryptedSize]bool{}
	for counter := 0; counter < encryptedSize; counter++ {
		r := bseRand(&seed)
		i := r & 0x3F
		for flags[i] {
			i = (i + 1) & 0x3F
		}

		r = bseRand(&seed)
		s := r & 0x07
		target := i

		k := bseRand(&seed)
		r = bseRand(&seed)

		plain := enc[target]
		var rotated uint8
		if k&1 != 0 {
			rotated = bits.RotateLeft8(plain, -int(s))
		} else {
			rotated = bits.RotateLeft8(plain, int(s))
		}
		enc[target] = uint8((uint16(rotated) + uint16(r)) & 0xFF)
		flags[i] = true
	}

	return out, nil
}

// bseRand implements the BSE LCG. It operates on 32-bit two's-complement
// values to match the C reference implementation.
func bseRand(seed *uint32) uint32 {
	x := *seed
	tmp := (((x * 257) >> 8) + x*97 + 23) ^ 0xA6D2D2D5
	*seed = ((tmp >> 16) & 0xFFFF) | (tmp << 16)
	return *seed & 0x7FFF
}
