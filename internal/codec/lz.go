package codec

import (
	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/binutil"
)

// DSC LZ parameters.
const (
	DSCDistanceBits = 12
	DSCMinDistance  = 2
	DSCMinMatchLen  = 3
	DSCMaxMatchLen  = 257
)

// DecodeDSCStyleLZ decodes the LZ scheme used inside DSC containers.
// Symbols 0-255 are literal bytes; symbols 256-511 encode a look-back
// of length (symbol - 254) (range 2..257). Each look-back is followed
// by a DSCDistanceBits distance value; real distance = value + DSCMinDistance.
func DecodeDSCStyleLZ(tree *Tree, bitstream []byte, outputSize int) ([]byte, error) {
	br := binutil.NewBitReader(bitstream)
	out := make([]byte, 0, outputSize)

	for len(out) < outputSize {
		sym, err := tree.Decode(br)
		if err != nil {
			return nil, err
		}
		if sym < 256 {
			out = append(out, byte(sym))
			continue
		}
		matchLen := sym - 254 // 256 -> 2, 511 -> 257
		if matchLen < 2 || matchLen > 257 {
			return nil, ErrInvalidHuffmanData
		}
		distBits, err := br.ReadBits(DSCDistanceBits)
		if err != nil {
			return nil, err
		}
		distance := int(distBits) + DSCMinDistance
		start := len(out) - distance
		if start < 0 {
			return nil, ErrInvalidHuffmanData
		}
		for i := 0; i < matchLen && len(out) < outputSize; i++ {
			out = append(out, out[start+i])
		}
	}
	if len(out) != outputSize {
		return nil, ErrInvalidHuffmanData
	}
	return out, nil
}
