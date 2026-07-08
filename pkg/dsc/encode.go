package dsc

import (
	"bytes"
	"fmt"
)

const (
	dscMaxDistance   = 4097
	dscMatchHashSize = 1 << 16
	dscMaxHashChecks = 64
)

type dscBitCode struct {
	bits []bool
}

type matchInfo struct {
	length  uint16
	distance uint16
}

type encodedToken struct {
	symbol   uint16
	distance uint16
	length   uint16
}

// msbBitWriter writes bits in most-significant-bit-first order.
type msbBitWriter struct {
	buf      *bytes.Buffer
	cur      byte
	freeBits uint8
}

func newMsbBitWriter(buf *bytes.Buffer) *msbBitWriter {
	return &msbBitWriter{buf: buf, freeBits: 8}
}

func (w *msbBitWriter) writeBits(bits []bool) {
	for _, bit := range bits {
		w.freeBits--
		if bit {
			w.cur |= 1 << w.freeBits
		}
		if w.freeBits == 0 {
			w.buf.WriteByte(w.cur)
			w.cur = 0
			w.freeBits = 8
		}
	}
}

func (w *msbBitWriter) writeValue(value uint32, bitCount int) {
	for i := bitCount - 1; i >= 0; i-- {
		w.freeBits--
		if (value>>uint(i))&1 != 0 {
			w.cur |= 1 << w.freeBits
		}
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

// buildCodeLengths constructs canonical Huffman code lengths from symbol frequencies.
func buildCodeLengths(frequencies []uint64) ([]uint8, error) {
	if len(frequencies) != dscSymbolCount {
		return nil, fmt.Errorf("invalid frequency table size %d", len(frequencies))
	}

	type node struct {
		freq  uint64
		order uint64
		sym   int
		left  int
		right int
	}
	nodes := make([]node, 0, dscSymbolCount*2)
	queue := make(priorityQueue, 0, dscSymbolCount)

	var order uint64
	for sym := 0; sym < dscSymbolCount; sym++ {
		if frequencies[sym] == 0 {
			continue
		}
		nodes = append(nodes, node{freq: frequencies[sym], order: order, sym: sym})
		queue.push(item{freq: frequencies[sym], order: order, index: len(nodes) - 1})
		order++
	}

	if queue.len() == 0 {
		return nil, fmt.Errorf("cannot encode an empty compiled script")
	}
	if queue.len() == 1 {
		lengths := make([]uint8, dscSymbolCount)
		it := queue.pop()
		lengths[nodes[it.index].sym] = 1
		return lengths, nil
	}

	for queue.len() > 1 {
		left := queue.pop()
		right := queue.pop()
		nodes = append(nodes, node{
			freq:  left.freq + right.freq,
			order: order,
			sym:   -1,
			left:  left.index,
			right: right.index,
		})
		queue.push(item{freq: left.freq + right.freq, order: order, index: len(nodes) - 1})
		order++
	}

	root := queue.pop().index
	lengths := make([]uint8, dscSymbolCount)
	var assign func(idx int, depth int)
	assign = func(idx int, depth int) {
		n := &nodes[idx]
		if n.sym >= 0 {
			if depth == 0 {
				depth = 1
			}
			lengths[n.sym] = uint8(depth)
			return
		}
		assign(n.left, depth+1)
		assign(n.right, depth+1)
	}
	assign(root, 0)
	return lengths, nil
}

type item struct {
	freq  uint64
	order uint64
	index int
}

type priorityQueue []item

func (q *priorityQueue) push(it item) {
	*q = append(*q, it)
	q.up(len(*q) - 1)
}

func (q *priorityQueue) pop() item {
	n := len(*q)
	q.swap(0, n-1)
	it := (*q)[n-1]
	*q = (*q)[:n-1]
	q.down(0)
	return it
}

func (q *priorityQueue) len() int { return len(*q) }

func (q *priorityQueue) up(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if q.less(i, p) {
			q.swap(i, p)
			i = p
		} else {
			break
		}
	}
}

func (q *priorityQueue) down(i int) {
	n := len(*q)
	for {
		left := 2*i + 1
		if left >= n {
			break
		}
		smallest := left
		right := left + 1
		if right < n && q.less(right, left) {
			smallest = right
		}
		if q.less(smallest, i) {
			q.swap(i, smallest)
			i = smallest
		} else {
			break
		}
	}
}

func (q *priorityQueue) swap(i, j int) { (*q)[i], (*q)[j] = (*q)[j], (*q)[i] }

func (q *priorityQueue) less(i, j int) bool {
	if (*q)[i].freq != (*q)[j].freq {
		return (*q)[i].freq < (*q)[j].freq
	}
	return (*q)[i].order < (*q)[j].order
}

func hashSequence(data []byte, position int) uint32 {
	return ((uint32(data[position]) << 16) ^ (uint32(data[position+1]) << 8) ^ uint32(data[position+2])) & (dscMatchHashSize - 1)
}

func findBestMatches(compiled []byte) []matchInfo {
	matches := make([]matchInfo, len(compiled))
	if len(compiled) < dscMinMatchLen {
		return matches
	}

	head := make([]int, dscMatchHashSize)
	for i := range head {
		head[i] = -1
	}
	previous := make([]int, len(compiled))
	for i := range previous {
		previous[i] = -1
	}

	for position := 0; position < len(compiled); position++ {
		if position+dscMinMatchLen > len(compiled) {
			continue
		}
		h := hashSequence(compiled, position)
		candidate := head[h]
		checks := 0
		best := matchInfo{}

		for candidate >= 0 && checks < dscMaxHashChecks {
			distance := position - candidate
			if distance < 2 {
				candidate = previous[candidate]
				checks++
				continue
			}
			if distance > dscMaxDistance {
				break
			}

			maxLen := len(compiled) - position
			if maxLen > dscMaxMatchLen {
				maxLen = dscMaxMatchLen
			}
			length := 0
			for length < maxLen && compiled[candidate+length] == compiled[position+length] {
				length++
			}

			if length >= dscMinMatchLen && length > int(best.length) {
				best.length = uint16(length)
				best.distance = uint16(distance)
				if length == dscMaxMatchLen {
					break
				}
			}
			candidate = previous[candidate]
			checks++
		}

		matches[position] = best
		previous[position] = head[h]
		head[h] = position
	}
	return matches
}

func tokenizeCompiled(compiled []byte, matches []matchInfo, estimatedBits []int) []encodedToken {
	const largeCost = int(^uint(0)>>1) / 4

	bestCost := make([]int, len(compiled)+1)
	bestLength := make([]uint16, len(compiled))
	bestDistance := make([]uint16, len(compiled))
	for i := range bestCost {
		bestCost[i] = largeCost
	}
	bestCost[len(compiled)] = 0

	for reverse := len(compiled); reverse > 0; reverse-- {
		pos := reverse - 1
		cost := estimatedBits[compiled[pos]] + bestCost[pos+1]
		bestLength[pos] = 1
		bestDistance[pos] = 0

		match := matches[pos]
		if match.length >= dscMinMatchLen {
			maxLen := int(match.length)
			if maxLen > dscMaxMatchLen {
				maxLen = dscMaxMatchLen
			}
			for length := dscMinMatchLen; length <= maxLen; length++ {
				symbol := 0x100 + length - 2
				candidateCost := estimatedBits[symbol] + 12 + bestCost[pos+length]
				if candidateCost < cost {
					cost = candidateCost
					bestLength[pos] = uint16(length)
					bestDistance[pos] = match.distance
				}
			}
		}
		bestCost[pos] = cost
	}

	var tokens []encodedToken
	for pos := 0; pos < len(compiled); {
		length := int(bestLength[pos])
		if length > 1 {
			tokens = append(tokens, encodedToken{
				symbol:   uint16(0x100 + length - 2),
				distance: bestDistance[pos],
				length:   uint16(length),
			})
			pos += length
		} else {
			tokens = append(tokens, encodedToken{symbol: uint16(compiled[pos]), length: 1})
			pos++
		}
	}
	return tokens
}

func countTokenFrequencies(tokens []encodedToken) []uint64 {
	freqs := make([]uint64, dscSymbolCount)
	for _, t := range tokens {
		freqs[t.symbol]++
	}
	return freqs
}

func buildEstimatedBitCosts(lengths []uint8) []int {
	estimated := make([]int, dscSymbolCount)
	for i := range estimated {
		estimated[i] = 9
	}
	for i, l := range lengths {
		if l != 0 {
			estimated[i] = int(l)
		}
	}
	return estimated
}

func optimizeTokens(compiled []byte) ([]encodedToken, []uint8, error) {
	matches := findBestMatches(compiled)
	estimatedBits := make([]int, dscSymbolCount)
	for i := range estimatedBits {
		estimatedBits[i] = 9
	}

	var tokens []encodedToken
	var lengths []uint8
	for iteration := 0; iteration < 4; iteration++ {
		candidate := tokenizeCompiled(compiled, matches, estimatedBits)
		freqs := countTokenFrequencies(candidate)
		var err error
		lengths, err = buildCodeLengths(freqs)
		if err != nil {
			return nil, nil, err
		}
		nextEstimated := buildEstimatedBitCosts(lengths)
		tokens = candidate
		same := true
		for i := 0; i < dscSymbolCount; i++ {
			if nextEstimated[i] != estimatedBits[i] {
				same = false
				break
			}
		}
		if same {
			break
		}
		estimatedBits = nextEstimated
	}
	return tokens, lengths, nil
}

func buildCodesFromTree(nodes [1024]dscNode) [dscSymbolCount]dscBitCode {
	var codes [dscSymbolCount]dscBitCode
	path := make([]bool, 0, 32)
	var visit func(nodeIndex uint32)
	visit = func(nodeIndex uint32) {
		node := &nodes[nodeIndex]
		if !node.hasChildren {
			symbol := uint16(node.value)
			if node.lookBehind {
				symbol |= 0x100
			}
			bits := make([]bool, len(path))
			copy(bits, path)
			codes[symbol].bits = bits
			if len(codes[symbol].bits) == 0 {
				codes[symbol].bits = []bool{false}
			}
			return
		}
		path = append(path, false)
		visit(node.children[0])
		path[len(path)-1] = true
		visit(node.children[1])
		path = path[:len(path)-1]
	}
	visit(0)
	return codes
}

// EncodeCompiledToDsc compresses compiled script bytes into a DSC container.
func EncodeCompiledToDsc(compiled []byte) ([]byte, error) {
	tokens, lengths, err := optimizeTokens(compiled)
	if err != nil {
		return nil, err
	}

	nodes, err := buildDscTree(lengths)
	if err != nil {
		return nil, err
	}
	codes := buildCodesFromTree(nodes)

	buf := &bytes.Buffer{}
	writer := newMsbBitWriter(buf)
	for _, token := range tokens {
		writer.writeBits(codes[token.symbol].bits)
		if token.length > 1 {
			writer.writeValue(uint32(token.distance-2), dscDistanceBits)
		}
	}
	bitstream := writer.finish()

	output := &bytes.Buffer{}
	output.Grow(len(dscMagic) + 16 + dscSymbolCount + len(bitstream))
	output.Write(dscMagic)
	const keySeed uint32 = 0
	writeU32LEBuffer(output, keySeed)
	writeU32LEBuffer(output, uint32(len(compiled)))
	writeU32LEBuffer(output, 0) // reserved
	writeU32LEBuffer(output, 0) // reserved

	key := keySeed
	for _, length := range lengths {
		output.WriteByte(length + nextKeyByte(&key))
	}
	output.Write(bitstream)
	return output.Bytes(), nil
}

func writeU32LEBuffer(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 24))
}
