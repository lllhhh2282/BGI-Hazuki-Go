package cbg

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/binutil"
	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/codec"
)

// DecodeCbg decodes a CBG image file into a RasterImage.
func DecodeCbg(path string) (*RasterImage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cbg: failed to read %s: %w", path, err)
	}
	img, err := DecodeCbgBytes(data)
	if err != nil {
		return nil, fmt.Errorf("cbg: %s: %w", path, err)
	}
	return img, nil
}

// DecodeCbgBytes decodes CBG image bytes into a RasterImage.
func DecodeCbgBytes(data []byte) (*RasterImage, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("cbg: file too small (%d bytes)", len(data))
	}
	if !bytes.Equal(data[:magicSize], magic) {
		return nil, ErrNotCbg
	}

	hdr, err := readHeader(data)
	if err != nil {
		return nil, err
	}
	if int(hdr.EncLength) > len(data)-headerSize {
		return nil, fmt.Errorf("cbg: encrypted length %d exceeds file size", hdr.EncLength)
	}

	encrypted := make([]byte, hdr.EncLength)
	copy(encrypted, data[headerSize:headerSize+int(hdr.EncLength)])
	actualSum, actualXor, err := decrypt(encrypted, hdr.Key)
	if err != nil {
		return nil, fmt.Errorf("cbg: decrypt failed: %w", err)
	}
	if actualSum != hdr.ChecksumSum || actualXor != hdr.ChecksumXor {
		return nil, fmt.Errorf("cbg: checksum mismatch")
	}

	compressed := data[headerSize+int(hdr.EncLength):]

	switch hdr.Version {
	case 1:
		return decodeV1(hdr, encrypted, compressed)
	case 2:
		return decodeV2(hdr, encrypted, data[headerSize+int(hdr.EncLength):])
	default:
		return nil, fmt.Errorf("cbg: unsupported version %d", hdr.Version)
	}
}

// ===================== CBG Version 1 (Huffman + RLE) =====================

type huffmanCode struct {
	bits     uint32
	bitCount int
	weight   uint32
}

type huffmanNode struct {
	isLeaf bool
	weight uint32
	value  int
	left   int
	right  int
	order  int
}

type huffmanTree struct {
	nodes []huffmanNode
	codes [256]huffmanCode
	root  int
}

// msbBitReader reads bits from a byte slice in big-endian bit order.
type msbBitReader struct {
	data      []byte
	bytePos   int
	bitPos    uint8 // 0 = MSB
	cache     byte
	cacheBits int
}

func newMsbBitReader(data []byte) *msbBitReader {
	return &msbBitReader{data: data}
}

func (r *msbBitReader) readBit() (int, error) {
	if r.cacheBits == 0 {
		if r.bytePos >= len(r.data) {
			return 0, binutil.ErrUnexpectedEOF
		}
		r.cache = r.data[r.bytePos]
		r.bytePos++
		r.cacheBits = 8
	}
	r.cacheBits--
	return int((r.cache >> uint(r.cacheBits)) & 1), nil
}

func (r *msbBitReader) readBits(n int) (int, error) {
	if n < 0 || n > 32 {
		return 0, fmt.Errorf("cbg: invalid bit count %d", n)
	}
	v := 0
	for i := 0; i < n; i++ {
		bit, err := r.readBit()
		if err != nil {
			return 0, err
		}
		v = (v << 1) | bit
	}
	return v, nil
}

func (r *msbBitReader) cacheBitsVal() int { return r.cacheBits }

func (r *msbBitReader) alignByte() {
	if r.cacheBits > 0 && r.cacheBits < 8 {
		r.cacheBits = 0
	}
}

// msbBitWriter writes bits in big-endian bit order.
type msbBitWriter struct {
	buf      *bytes.Buffer
	cur      byte
	freeBits uint8
}

func newMsbBitWriter(buf *bytes.Buffer) *msbBitWriter {
	return &msbBitWriter{buf: buf, freeBits: 8}
}

func (w *msbBitWriter) write(value uint32, bitCount int) {
	for bitCount > 0 {
		chunk := int(w.freeBits)
		if bitCount < chunk {
			chunk = bitCount
		}
		mask := uint32((1 << chunk) - 1)
		bitCount -= chunk
		w.freeBits -= uint8(chunk)
		w.cur |= byte(((value >> uint(bitCount)) & mask) << w.freeBits)
		if w.freeBits == 0 {
			w.buf.WriteByte(w.cur)
			w.cur = 0
			w.freeBits = 8
		}
	}
}

func (w *msbBitWriter) finish() []byte {
	if w.freeBits != 8 {
		w.buf.WriteByte(w.cur)
		w.cur = 0
		w.freeBits = 8
	}
	return w.buf.Bytes()
}

func buildHuffmanTree(frequencies [256]uint32) *huffmanTree {
	tree := &huffmanTree{root: -1}
	tree.nodes = make([]huffmanNode, 0, 512)
	active := make([]int, 0, 256)

	for sym := 0; sym < 256; sym++ {
		if frequencies[sym] == 0 {
			continue
		}
		tree.nodes = append(tree.nodes, huffmanNode{
			isLeaf: true,
			weight: frequencies[sym],
			value:  sym,
			order:  sym,
		})
		active = append(active, len(tree.nodes)-1)
	}
	if len(active) == 0 {
		return nil
	}

	nextOrder := 256
	for len(active) > 1 {
		sort.Slice(active, func(i, j int) bool {
			a := &tree.nodes[active[i]]
			b := &tree.nodes[active[j]]
			if a.weight != b.weight {
				return a.weight < b.weight
			}
			return a.order < b.order
		})
		left := active[0]
		right := active[1]
		active = active[2:]

		parent := huffmanNode{
			isLeaf: false,
			weight: tree.nodes[left].weight + tree.nodes[right].weight,
			left:   left,
			right:  right,
			order:  nextOrder,
		}
		nextOrder++
		tree.nodes = append(tree.nodes, parent)
		active = append(active, len(tree.nodes)-1)
	}
	tree.root = active[0]

	var assign func(nodeIndex int, bits uint32, bitCount int)
	assign = func(nodeIndex int, bits uint32, bitCount int) {
		node := &tree.nodes[nodeIndex]
		if node.isLeaf {
			tree.codes[node.value] = huffmanCode{
				bits:     bits,
				bitCount: bitCount,
				weight:   node.weight,
			}
			return
		}
		assign(node.left, bits<<1, bitCount+1)
		assign(node.right, (bits<<1)|1, bitCount+1)
	}
	assign(tree.root, 0, 0)
	return tree
}

func huffmanDecode(bitstream []byte, outputSize int, tree *huffmanTree) ([]byte, error) {
	if outputSize < 0 {
		return nil, fmt.Errorf("cbg: invalid decode size")
	}
	out := make([]byte, 0, outputSize)
	if tree.nodes[tree.root].isLeaf {
		for i := 0; i < outputSize; i++ {
			out = append(out, byte(tree.nodes[tree.root].value))
		}
		return out, nil
	}

	reader := newMsbBitReader(bitstream)
	for len(out) < outputSize {
		idx := tree.root
		for !tree.nodes[idx].isLeaf {
			bit, err := reader.readBit()
			if err != nil {
				return nil, fmt.Errorf("cbg: huffman decode truncated: %w", err)
			}
			if bit == 0 {
				idx = tree.nodes[idx].left
			} else {
				idx = tree.nodes[idx].right
			}
			if idx < 0 {
				return nil, fmt.Errorf("cbg: invalid huffman branch")
			}
		}
		out = append(out, byte(tree.nodes[idx].value))
	}
	return out, nil
}

func huffmanEncode(input []byte, tree *huffmanTree) []byte {
	buf := &bytes.Buffer{}
	writer := newMsbBitWriter(buf)
	for _, v := range input {
		code := tree.codes[v]
		writer.write(code.bits, code.bitCount)
	}
	return writer.finish()
}

func readFrequencies(data []byte) ([256]uint32, error) {
	var freqs [256]uint32
	r := bytes.NewReader(data)
	for i := range freqs {
		v, err := binutil.ReadVarInt(r)
		if err != nil {
			return freqs, fmt.Errorf("cbg: failed to read frequency %d: %w", i, err)
		}
		freqs[i] = v
	}
	return freqs, nil
}

func reverseAverageSampling(pixels []byte, width, height, channels uint32) {
	if len(pixels) == 0 {
		return
	}
	stride := int(width) * int(channels)

	// First row.
	for x := 1; x < int(width); x++ {
		idx := x * int(channels)
		for c := 0; c < int(channels); c++ {
			pixels[idx+c] += pixels[idx-int(channels)+c]
		}
	}

	for y := 1; y < int(height); y++ {
		rowOff := y * stride
		for c := 0; c < int(channels); c++ {
			pixels[rowOff+c] += pixels[rowOff-stride+c]
		}
		for x := 1; x < int(width); x++ {
			idx := rowOff + x*int(channels)
			for c := 0; c < int(channels); c++ {
				left := pixels[idx-int(channels)+c]
				above := pixels[idx-stride+c]
				pixels[idx+c] += byte((int(left) + int(above)) >> 1)
			}
		}
	}
}

func unpackPixels(packed []byte, width, height, channels uint32) *RasterImage {
	img := &RasterImage{
		Width:  width,
		Height: height,
		Pixels: make([]uint8, width*height*4),
	}
	for i := range img.Pixels {
		img.Pixels[i] = 0xFF
	}

	src := 0
	for pixel := uint32(0); pixel < width*height; pixel++ {
		dst := pixel * 4
		switch channels {
		case 1:
			img.Pixels[dst+0] = packed[src]
			img.Pixels[dst+1] = packed[src]
			img.Pixels[dst+2] = packed[src]
			src++
		case 2:
			img.Pixels[dst+0] = packed[src+0]
			img.Pixels[dst+1] = packed[src+1]
			img.Pixels[dst+2] = 0
			src += 2
		case 3:
			img.Pixels[dst+0] = packed[src+0]
			img.Pixels[dst+1] = packed[src+1]
			img.Pixels[dst+2] = packed[src+2]
			src += 3
		case 4:
			img.Pixels[dst+0] = packed[src+0]
			img.Pixels[dst+1] = packed[src+1]
			img.Pixels[dst+2] = packed[src+2]
			img.Pixels[dst+3] = packed[src+3]
			src += 4
		}
	}

	if channels == 4 {
		img.HasAlpha = false
		for i := 3; i < len(img.Pixels); i += 4 {
			if img.Pixels[i] != 0xFF {
				img.HasAlpha = true
				break
			}
		}
	}
	return img
}

func decodeV1(hdr *cbgHeader, decrypted, compressed []byte) (*RasterImage, error) {
	if hdr.BitsPerPixel != 8 && hdr.BitsPerPixel != 16 && hdr.BitsPerPixel != 24 && hdr.BitsPerPixel != 32 {
		return nil, fmt.Errorf("cbg v1: unsupported bits per pixel %d", hdr.BitsPerPixel)
	}
	channels := hdr.BitsPerPixel / 8

	freqs, err := readFrequencies(decrypted)
	if err != nil {
		return nil, fmt.Errorf("cbg v1: %w", err)
	}
	tree := buildHuffmanTree(freqs)
	if tree == nil {
		return nil, fmt.Errorf("cbg v1: failed to build huffman tree")
	}

	rleData, err := huffmanDecode(compressed, int(hdr.IntermediateLength), tree)
	if err != nil {
		return nil, fmt.Errorf("cbg v1: huffman decode failed: %w", err)
	}

	expectedPacked := int(hdr.Width) * int(hdr.Height) * int(channels)
	packed, err := codec.DecodeRLE(rleData, expectedPacked)
	if err != nil {
		return nil, fmt.Errorf("cbg v1: RLE decode failed: %w", err)
	}

	reverseAverageSampling(packed, uint32(hdr.Width), uint32(hdr.Height), channels)
	return unpackPixels(packed, uint32(hdr.Width), uint32(hdr.Height), channels), nil
}

// ===================== CBG Version 2 (DCT-based) =====================

var kDctBasisTable = [64]float32{
	1.00000000, 1.38703990, 1.30656302, 1.17587554, 1.00000000, 0.78569496, 0.54119611, 0.27589938,
	1.38703990, 1.92387950, 1.81225491, 1.63098633, 1.38703990, 1.08979023, 0.75066054, 0.38268343,
	1.30656302, 1.81225491, 1.70710683, 1.53635550, 1.30656302, 1.02655995, 0.70710677, 0.36047992,
	1.17587554, 1.63098633, 1.53635550, 1.38268340, 1.17587554, 0.92387950, 0.63637930, 0.32442334,
	1.00000000, 1.38703990, 1.30656302, 1.17587554, 1.00000000, 0.78569496, 0.54119611, 0.27589938,
	0.78569496, 1.08979023, 1.02655995, 0.92387950, 0.78569496, 0.61731654, 0.42521504, 0.21677275,
	0.54119611, 0.75066054, 0.70710677, 0.63637930, 0.54119611, 0.42521504, 0.29289323, 0.14931567,
	0.27589938, 0.38268343, 0.36047992, 0.32442334, 0.27589938, 0.21677275, 0.14931567, 0.07612047,
}

var kBlockFillOrder = [64]uint8{
	0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34, 27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
}

type huffmanTreeV2 struct {
	nodes []huffmanNodeV2
}

type huffmanNodeV2 struct {
	valid    bool
	isParent bool
	weight   uint32
	left     int
	right    int
}

func newHuffmanTreeV2(weights []uint32) *huffmanTreeV2 {
	tree := &huffmanTreeV2{}
	tree.nodes = make([]huffmanNodeV2, 0, len(weights)*2)
	var rootWeight uint32
	for _, w := range weights {
		tree.nodes = append(tree.nodes, huffmanNodeV2{
			valid:  w != 0,
			weight: w,
		})
		rootWeight += w
	}
	if rootWeight == 0 {
		return nil
	}

	for {
		var combined uint32
		children := [2]int{-1, -1}
		for c := 0; c < 2; c++ {
			minW := ^uint32(0)
			idx := -1
			n := 0
			for ; n < len(tree.nodes); n++ {
				if tree.nodes[n].valid {
					minW = tree.nodes[n].weight
					idx = n
					n++
					break
				}
			}
			if n < c+1 {
				n = c + 1
			}
			for ; n < len(tree.nodes); n++ {
				if tree.nodes[n].valid && tree.nodes[n].weight < minW {
					minW = tree.nodes[n].weight
					idx = n
				}
			}
			if idx >= 0 {
				tree.nodes[idx].valid = false
				combined += tree.nodes[idx].weight
				children[c] = idx
			}
		}
		tree.nodes = append(tree.nodes, huffmanNodeV2{
			valid:    true,
			isParent: true,
			weight:   combined,
			left:     children[0],
			right:    children[1],
		})
		if combined >= rootWeight {
			break
		}
	}
	return tree
}

func (t *huffmanTreeV2) decodeToken(reader *msbBitReader) (int, error) {
	idx := len(t.nodes) - 1
	for t.nodes[idx].isParent {
		bit, err := reader.readBit()
		if err != nil {
			return 0, err
		}
		if bit == 0 {
			idx = t.nodes[idx].left
		} else {
			idx = t.nodes[idx].right
		}
		if idx < 0 {
			return 0, fmt.Errorf("cbg v2: invalid huffman tree node")
		}
	}
	return idx, nil
}

func readWeightTableV2(data []byte, pos *int, count int) ([]uint32, error) {
	weights := make([]uint32, count)
	for i := 0; i < count; i++ {
		var val uint32
		var shift uint32
		for {
			if *pos >= len(data) {
				return nil, fmt.Errorf("cbg v2: unexpected end of weight table")
			}
			b := data[*pos]
			*pos++
			val |= uint32(b&0x7F) << shift
			if (b & 0x80) == 0 {
				break
			}
			shift += 7
			if shift >= 32 {
				return nil, fmt.Errorf("cbg v2: varint overflow in weight table")
			}
		}
		weights[i] = val
	}
	return weights, nil
}

func floatToShort(f float32) int16 {
	a := 0x80 + (int(f) >> 3)
	if a <= 0 {
		return 0
	}
	if a <= 0xFF {
		return int16(a)
	}
	if a < 0x180 {
		return 0xFF
	}
	return 0
}

func floatToByte(f float32) uint8 {
	if f >= 255.0 {
		return 0xFF
	}
	if f <= 0.0 {
		return 0
	}
	return uint8(f)
}

func decodeDCT(channel int, data []int16, src int, dct [2][64]float32, ycbcr *[64][3]int16, tmp *[8][8]float32) {
	d := 0
	if channel > 0 {
		d = 1
	}
	for i := 0; i < 8; i++ {
		if data[src+8+i] == 0 && data[src+16+i] == 0 && data[src+24+i] == 0 &&
			data[src+32+i] == 0 && data[src+40+i] == 0 && data[src+48+i] == 0 &&
			data[src+56+i] == 0 {
			t := float32(data[src+i]) * dct[d][i]
			for j := 0; j < 8; j++ {
				tmp[j][i] = t
			}
			continue
		}

		v1 := float32(data[src+i]) * dct[d][i]
		v2 := float32(data[src+8+i]) * dct[d][8+i]
		v3 := float32(data[src+16+i]) * dct[d][16+i]
		v4 := float32(data[src+24+i]) * dct[d][24+i]
		v5 := float32(data[src+32+i]) * dct[d][32+i]
		v6 := float32(data[src+40+i]) * dct[d][40+i]
		v7 := float32(data[src+48+i]) * dct[d][48+i]
		v8 := float32(data[src+56+i]) * dct[d][56+i]

		v10 := v1 + v5
		v11 := v1 - v5
		v12 := v3 + v7
		v13 := (v3-v7)*1.414213562 - v12
		v3_ := v11 + v13
		v5_ := v11 - v13
		v7_ := v10 - v12
		v1_ := v10 + v12

		v14 := v2 + v8
		v15 := v2 - v8
		v16 := v6 + v4
		v17 := v6 - v4
		v8_ := v14 + v16
		v11_ := (v14 - v16) * 1.414213562
		v9 := (v17 + v15) * 1.847759065
		v10_ := 1.082392200*v15 - v9
		v13_ := -2.613125930*v17 + v9
		v6_ := v13_ - v8_
		v4_ := v11_ - v6_
		v2_ := v10_ + v4_

		tmp[0][i] = v1_ + v8_
		tmp[1][i] = v3_ + v6_
		tmp[2][i] = v5_ + v4_
		tmp[3][i] = v7_ - v2_
		tmp[4][i] = v7_ + v2_
		tmp[5][i] = v5_ - v4_
		tmp[6][i] = v3_ - v6_
		tmp[7][i] = v1_ - v8_
	}

	dst := 0
	for i := 0; i < 8; i++ {
		v10 := tmp[i][0] + tmp[i][4]
		v11 := tmp[i][0] - tmp[i][4]
		v12 := tmp[i][2] + tmp[i][6]
		v13 := (tmp[i][2]-tmp[i][6])*1.414213562 - v12
		v14 := tmp[i][1] + tmp[i][7]
		v15 := tmp[i][1] - tmp[i][7]
		v16 := tmp[i][5] + tmp[i][3]
		v17 := tmp[i][5] - tmp[i][3]

		v1 := v10 + v12
		v7 := v10 - v12
		v3 := v11 + v13
		v5 := v11 - v13
		v8 := v14 + v16
		v11_ := (v14 - v16) * 1.414213562
		v9 := (v17 + v15) * 1.847759065
		v10_ := v9 - v15*1.082392200
		v13_ := v9 - v17*2.613125930
		v6 := v13_ - v8
		v4 := v11_ - v6
		v2 := v10_ - v4

		ycbcr[dst][channel] = floatToShort(v1 + v8)
		dst++
		ycbcr[dst][channel] = floatToShort(v3 + v6)
		dst++
		ycbcr[dst][channel] = floatToShort(v5 + v4)
		dst++
		ycbcr[dst][channel] = floatToShort(v7 + v2)
		dst++
		ycbcr[dst][channel] = floatToShort(v7 - v2)
		dst++
		ycbcr[dst][channel] = floatToShort(v5 - v4)
		dst++
		ycbcr[dst][channel] = floatToShort(v3 - v6)
		dst++
		ycbcr[dst][channel] = floatToShort(v1 - v8)
		dst++
	}
}

func decodeRGBBlocks(blockData []byte, blockSize int, tree1, tree2 *huffmanTreeV2,
	dct [2][64]float32, width int, output []uint8, dstOffset int) error {
	if blockSize <= 0 {
		return nil
	}
	pos := 0
	blockCountVal := uint32(0)
	{
		var shift uint32
		for pos < blockSize {
			b := blockData[pos]
			pos++
			blockCountVal |= uint32(b&0x7F) << shift
			if (b & 0x80) == 0 {
				break
			}
			shift += 7
		}
	}
	if blockCountVal == 0 {
		return nil
	}

	colorData := make([]int16, blockCountVal)
	bits := newMsbBitReader(blockData[pos:blockSize])

	// Decode DC coefficients.
	acc := 0
	for i := uint32(0); i < blockCountVal; i += 64 {
		count, err := tree1.decodeToken(bits)
		if err != nil {
			return fmt.Errorf("cbg v2: DC decode failed: %w", err)
		}
		if count != 0 {
			v, err := bits.readBits(count)
			if err != nil {
				return fmt.Errorf("cbg v2: DC bits failed: %w", err)
			}
			if v >= 0 && count > 0 && (v>>(count-1)) == 0 {
				v = ((-1 << count) | v) + 1
			}
			acc += v
		}
		colorData[i] = int16(acc)
	}

	// Align to byte boundary.
	if bits.cacheBitsVal()&7 != 0 {
		bits.readBits(bits.cacheBitsVal() & 7)
	}

	// Decode AC coefficients.
	for i := uint32(0); i < blockCountVal; i += 64 {
		index := 1
		for index < 64 {
			code, err := tree2.decodeToken(bits)
			if err != nil {
				return fmt.Errorf("cbg v2: AC decode failed: %w", err)
			}
			if code == 0 {
				break
			}
			if code == 0xF {
				index += 0x10
				continue
			}
			index += code & 0xF
			if index >= 64 {
				break
			}
			nbits := code >> 4
			v, err := bits.readBits(nbits)
			if err != nil {
				return fmt.Errorf("cbg v2: AC bits failed: %w", err)
			}
			if nbits != 0 && v >= 0 && (v>>(nbits-1)) == 0 {
				v = ((-1 << nbits) | v) + 1
			}
			colorData[i+uint32(kBlockFillOrder[index])] = int16(v)
			index++
		}
	}

	blockCountX := width / 8
	blkDst := dstOffset
	var ycbcr [64][3]int16
	var tmp [8][8]float32
	for bx := 0; bx < blockCountX; bx++ {
		src := bx * 64
		for ch := 0; ch < 3; ch++ {
			decodeDCT(ch, colorData, src, dct, &ycbcr, &tmp)
			src += width * 8
		}
		for j := 0; j < 64; j++ {
			cy := float32(ycbcr[j][0])
			cb := float32(ycbcr[j][1])
			cr := float32(ycbcr[j][2])
			r := cy + 1.402*cr - 178.956
			g := cy - 0.34414*cb - 0.71414*cr + 135.95984
			b := cy + 1.772*cb - 226.316
			y := j >> 3
			x := j & 7
			p := blkDst + (y*width+x)*4
			output[p+0] = floatToByte(b)
			output[p+1] = floatToByte(g)
			output[p+2] = floatToByte(r)
			output[p+3] = 0xFF
		}
		blkDst += 32
	}
	return nil
}

func decodeAlphaV2(data []byte, size int, width int, output []uint8, outputSize int) {
	if size < 4 {
		return
	}
	marker := uint32(data[0]) | (uint32(data[1]) << 8) | (uint32(data[2]) << 16) | (uint32(data[3]) << 24)
	if marker != 1 {
		return
	}
	src := 4
	dst := 3
	ctl := 1 << 1
	for dst < outputSize && src < size {
		ctl >>= 1
		if ctl == 1 {
			if src >= size {
				break
			}
			ctl = int(data[src]) | 0x100
			src++
		}
		if ctl&1 != 0 {
			if src+1 >= size {
				break
			}
			v := int(data[src]) | (int(data[src+1]) << 8)
			src += 2
			xoff := v & 0x3F
			if xoff > 0x1F {
				xoff |= ^0x3F
			}
			yoff := (v >> 6) & 7
			if yoff != 0 {
				yoff |= ^7
			}
			count := ((v >> 9) & 0x7F) + 3
			ref := dst + (xoff+yoff*width)*4
			if ref >= dst {
				break
			}
			for i := 0; i < count && dst < outputSize; i++ {
				if ref >= 0 && ref < outputSize {
					output[dst] = output[ref]
				}
				ref += 4
				dst += 4
			}
		} else {
			if src >= size {
				break
			}
			output[dst] = data[src]
			src++
			dst += 4
		}
	}
}

func decodeV2(hdr *cbgHeader, decrypted, stream []byte) (*RasterImage, error) {
	if hdr.BitsPerPixel != 24 && hdr.BitsPerPixel != 32 {
		return nil, fmt.Errorf("cbg v2: unsupported bits per pixel %d (only 24/32 implemented)", hdr.BitsPerPixel)
	}
	if len(decrypted) < 0x80 {
		return nil, fmt.Errorf("cbg v2: encrypted data too short for DCT coefficients")
	}

	var dct [2][64]float32
	for i := 0; i < 0x80; i++ {
		dct[i>>6][i&0x3F] = float32(decrypted[i]) * kDctBasisTable[i&0x3F]
	}

	width := int(hdr.Width)
	height := int(hdr.Height)
	paddedW := (width + 7) & ^7
	paddedH := (height + 7) & ^7

	pos := 0
	baseOffset := 0

	weights1, err := readWeightTableV2(stream, &pos, 0x10)
	if err != nil {
		return nil, fmt.Errorf("cbg v2: %w", err)
	}
	weights2, err := readWeightTableV2(stream, &pos, 0xB0)
	if err != nil {
		return nil, fmt.Errorf("cbg v2: %w", err)
	}

	tree1 := newHuffmanTreeV2(weights1)
	tree2 := newHuffmanTreeV2(weights2)
	if tree1 == nil || tree2 == nil {
		return nil, fmt.Errorf("cbg v2: failed to build huffman trees")
	}

	yBlocks := paddedH / 8
	offsets := make([]int32, yBlocks+1)
	inputBase := (pos + (yBlocks+1)*4) - baseOffset
	for i := 0; i <= yBlocks; i++ {
		if pos+4 > len(stream) {
			return nil, fmt.Errorf("cbg v2: unexpected end reading block offsets")
		}
		off := int32(stream[pos]) | (int32(stream[pos+1]) << 8) |
			(int32(stream[pos+2]) << 16) | (int32(stream[pos+3]) << 24)
		offsets[i] = off - int32(inputBase)
		pos += 4
	}

	blockBase := stream[pos:]
	blockTotalSize := len(stream) - pos
	padSkip := ((paddedW >> 3) + 7) >> 3

	output := make([]uint8, paddedW*paddedH*4)
	for i := range output {
		output[i] = 0xFF
	}

	for i := 0; i < yBlocks; i++ {
		blockOffset := int(offsets[i]) + padSkip
		nextOffset := blockTotalSize
		if i+1 < yBlocks {
			nextOffset = int(offsets[i+1])
		}
		if blockOffset < 0 || blockOffset >= blockTotalSize {
			continue
		}
		blkLen := nextOffset - blockOffset
		if blkLen <= 0 {
			continue
		}
		dst := i * paddedW * 8 * 4
		if err := decodeRGBBlocks(blockBase[blockOffset:blockOffset+blkLen], blkLen,
			tree1, tree2, dct, paddedW, output, dst); err != nil {
			return nil, err
		}
	}

	hasAlpha := false
	if hdr.BitsPerPixel == 32 {
		alphaOffset := int(offsets[yBlocks])
		if alphaOffset >= 0 && alphaOffset < blockTotalSize {
			decodeAlphaV2(blockBase[alphaOffset:], blockTotalSize-alphaOffset,
				paddedW, output, len(output))
			for pi := 3; pi < len(output); pi += 4 {
				if output[pi] != 0xFF {
					hasAlpha = true
					break
				}
			}
		}
	}

	img := &RasterImage{
		Width:    uint32(width),
		Height:   uint32(height),
		HasAlpha: hasAlpha,
		Pixels:   make([]uint8, width*height*4),
	}
	if paddedW == width && paddedH == height {
		copy(img.Pixels, output)
	} else {
		for y := 0; y < height; y++ {
			srcRow := y * paddedW * 4
			dstRow := y * width * 4
			copy(img.Pixels[dstRow:dstRow+width*4], output[srcRow:srcRow+width*4])
		}
	}
	return img, nil
}
