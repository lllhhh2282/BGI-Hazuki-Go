package codec

import (
	"bytes"
	"errors"

	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/binutil"
)

// ErrInvalidRLE is returned when RLE data is malformed.
var ErrInvalidRLE = errors.New("invalid RLE data")

// DecodeRLE decodes the CBG v1 RLE stream.
// Format alternates: [nonzero_len:varint] [nonzero_bytes] [zero_len:varint] ...
func DecodeRLE(data []byte, expectedLen int) ([]byte, error) {
	if expectedLen < 0 {
		return nil, ErrInvalidRLE
	}
	r := bytes.NewReader(data)
	out := make([]byte, 0, expectedLen)

	for len(out) < expectedLen {
		nonzeroLen, err := binutil.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		if int(nonzeroLen) > expectedLen-len(out) {
			return nil, ErrInvalidRLE
		}
		buf := make([]byte, nonzeroLen)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		out = append(out, buf...)
		if len(out) >= expectedLen {
			break
		}
		zeroLen, err := binutil.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		if int(zeroLen) > expectedLen-len(out) {
			return nil, ErrInvalidRLE
		}
		out = append(out, make([]byte, zeroLen)...)
	}
	if len(out) != expectedLen {
		return nil, ErrInvalidRLE
	}
	return out, nil
}

// EncodeRLE encodes data using the CBG v1 RLE scheme.
// Only zero runs longer than 4 are compressed; shorter runs are kept as nonzero data.
func EncodeRLE(data []byte) []byte {
	const minZeroRun = 4
	var out bytes.Buffer
	i := 0
	for i < len(data) {
		if data[i] == 0 {
			// measure zero run
			j := i
			for j < len(data) && data[j] == 0 {
				j++
			}
			zeroRun := j - i
			if zeroRun > minZeroRun {
				binutil.WriteVarInt(&out, 0) // nonzero length 0
				binutil.WriteVarInt(&out, uint32(zeroRun))
				i = j
				continue
			}
		}
		// collect nonzero run
		j := i
		for j < len(data) {
			if data[j] == 0 {
				// check if it's worth splitting
				z := j
				for z < len(data) && data[z] == 0 {
					z++
				}
				if z-j > minZeroRun {
					break
				}
				j = z
				continue
			}
			j++
		}
		nonzeroRun := j - i
		binutil.WriteVarInt(&out, uint32(nonzeroRun))
		out.Write(data[i:j])
		i = j
	}
	return out.Bytes()
}
