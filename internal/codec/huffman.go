// Package codec provides reusable compression primitives used by BGI formats.
package codec

import (
	"container/heap"
	"errors"

	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/binutil"
)

// ErrInvalidHuffmanData is returned when huffman data is malformed.
var ErrInvalidHuffmanData = errors.New("invalid huffman data")

// TreeNode is a node in a huffman decoding tree.
// Leaf nodes have Symbol >= 0; internal nodes have Symbol == -1.
type TreeNode struct {
	Symbol int
	Left   *TreeNode
	Right  *TreeNode
}

// Tree is a huffman decoding tree with a fast lookup table for short codes.
type Tree struct {
	root      *TreeNode
	minLen    int
	maxLen    int
	lut       []int   // lookup table: index = code, value = symbol or -1 if miss
	lutBits   int
	lutMiss   []int   // for codes longer than lutBits, stores next node index or -symbol-2
}

// NewTreeFromFrequencies builds a huffman tree from symbol frequencies.
func NewTreeFromFrequencies(freqs []uint32) (*Tree, error) {
	if len(freqs) == 0 {
		return nil, ErrInvalidHuffmanData
	}
	h := &freqHeap{}
	for sym, f := range freqs {
		if f > 0 {
			heap.Push(h, &heapItem{freq: f, node: &TreeNode{Symbol: sym}})
		}
	}
	if h.Len() == 0 {
		return nil, ErrInvalidHuffmanData
	}
	heap.Init(h)
	for h.Len() > 1 {
		a := heap.Pop(h).(*heapItem)
		b := heap.Pop(h).(*heapItem)
		parent := &TreeNode{Symbol: -1, Left: a.node, Right: b.node}
		heap.Push(h, &heapItem{freq: a.freq + b.freq, node: parent})
	}
	root := heap.Pop(h).(*heapItem).node
	return buildTree(root)
}

// NewTreeFromLengths builds a huffman tree from per-symbol code lengths (DSC style).
// length[sym] == 0 means the symbol is unused.
func NewTreeFromLengths(lengths []uint8) (*Tree, error) {
	if len(lengths) == 0 {
		return nil, ErrInvalidHuffmanData
	}
	// Count codes per length and find max length.
	var maxLen int
	count := make([]int, 17)
	for _, l := range lengths {
		if l > 16 {
			return nil, ErrInvalidHuffmanData
		}
		count[l]++
		if int(l) > maxLen {
			maxLen = int(l)
		}
	}
	if maxLen == 0 {
		return nil, ErrInvalidHuffmanData
	}

	// Compute first code of each length (canonical huffman).
	code := 0
	nextCode := make([]int, maxLen+1)
	for l := 1; l <= maxLen; l++ {
		code = (code + count[l-1]) << 1
		nextCode[l] = code
	}

	root := &TreeNode{Symbol: -1}
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		c := nextCode[l]
		nextCode[l]++
		node := root
		for bit := l - 1; bit >= 0; bit-- {
			if (c>>bit)&1 == 0 {
				if node.Left == nil {
					node.Left = &TreeNode{Symbol: -1}
				}
				node = node.Left
			} else {
				if node.Right == nil {
					node.Right = &TreeNode{Symbol: -1}
				}
				node = node.Right
			}
		}
		if node.Symbol != -1 {
			return nil, ErrInvalidHuffmanData
		}
		node.Symbol = sym
	}
	return buildTree(root)
}

// Decode reads bits from br and returns the decoded symbol.
func (t *Tree) Decode(br *binutil.BitReader) (int, error) {
	if t.root == nil {
		return 0, ErrInvalidHuffmanData
	}
	node := t.root
	for node.Symbol == -1 {
		bit, err := br.ReadBit()
		if err != nil {
			return 0, err
		}
		if bit == 0 {
			node = node.Left
		} else {
			node = node.Right
		}
		if node == nil {
			return 0, ErrInvalidHuffmanData
		}
	}
	return node.Symbol, nil
}

// buildTree validates the tree and computes metadata.
func buildTree(root *TreeNode) (*Tree, error) {
	minLen, maxLen := -1, 0
	var walk func(n *TreeNode, depth int)
	walk = func(n *TreeNode, depth int) {
		if n == nil {
			return
		}
		if n.Symbol >= 0 {
			if minLen == -1 || depth < minLen {
				minLen = depth
			}
			if depth > maxLen {
				maxLen = depth
			}
			return
		}
		walk(n.Left, depth+1)
		walk(n.Right, depth+1)
	}
	walk(root, 0)
	if minLen == -1 {
		return nil, ErrInvalidHuffmanData
	}
	t := &Tree{root: root, minLen: minLen, maxLen: maxLen}
	return t, nil
}

type heapItem struct {
	freq uint32
	node *TreeNode
}

type freqHeap []*heapItem

func (h freqHeap) Len() int            { return len(h) }
func (h freqHeap) Less(i, j int) bool  { return h[i].freq < h[j].freq }
func (h freqHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *freqHeap) Push(x interface{}) { *h = append(*h, x.(*heapItem)) }
func (h *freqHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
