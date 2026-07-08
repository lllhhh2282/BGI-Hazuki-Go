// Package arc implements reading and extracting BGI ARC archives.
package arc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

	"github.com/lllhhh2282/BGI-Hazuki-Go/internal/binutil"
)

const headerSize = 16

var (
	magicV1 = []byte("PackFile    ")
	magicV2 = []byte("BURIKO ARC20")
)

// Entry describes one file inside an ARC archive.
type Entry struct {
	RelativePath string
	Offset       uint32
	Size         uint32
}

// ArchiveInfo holds metadata of an ARC archive.
type ArchiveInfo struct {
	ArcPath string
	Version int // 1 or 2
	Entries []Entry
}

// EntryCallback is called for each successfully extracted entry.
// index is 1-based, total is the total number of entries.
type EntryCallback func(entry Entry, outputPath string, index, total int)

// IsBurikoArcFile reports whether path points to a supported ARC archive
// (either the v1 "PackFile" format or the v2 "BURIKO ARC20" format).
func IsBurikoArcFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	hdr := make([]byte, headerSize)
	if _, err := f.Read(hdr); err != nil {
		return false, nil // too small or unreadable -> not an arc
	}
	return isArcHeader(hdr), nil
}

func isArcHeader(hdr []byte) bool {
	return len(hdr) >= headerSize &&
		(bytes.Equal(hdr[:len(magicV1)], magicV1) || bytes.Equal(hdr[:len(magicV2)], magicV2))
}

// ReadArcArchiveInfo reads the entry table of an ARC archive without extracting.
func ReadArcArchiveInfo(path string) (*ArchiveInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, err := parseArchive(data)
	if err != nil {
		return nil, err
	}
	info.ArcPath = path
	return info, nil
}

// ExtractArcArchive extracts all entries from arcPath into outputDir.
// The optional callback is invoked after each successful entry.
func ExtractArcArchive(arcPath, outputDir string, cb EntryCallback) ([]string, error) {
	data, err := os.ReadFile(arcPath)
	if err != nil {
		return nil, err
	}
	info, err := parseArchive(data)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}

	baseOffset := int64(headerSize) + int64(len(info.Entries))*int64(entrySizeForVersion(info.Version))
	extracted := make([]string, 0, len(info.Entries))

	for i, e := range info.Entries {
		absOffset := baseOffset + int64(e.Offset)
		endOffset := absOffset + int64(e.Size)
		if endOffset > int64(len(data)) {
			return nil, fmt.Errorf("entry %d exceeds file bounds", i)
		}

		outPath := filepath.Join(outputDir, e.RelativePath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(outPath, data[absOffset:endOffset], 0o644); err != nil {
			return nil, err
		}
		extracted = append(extracted, outPath)
		if cb != nil {
			cb(e, outPath, i+1, len(info.Entries))
		}
	}
	return extracted, nil
}

func parseArchive(data []byte) (*ArchiveInfo, error) {
	if len(data) < headerSize {
		return nil, errors.New("arc: file too small for header")
	}

	var version int
	switch {
	case bytes.Equal(data[:len(magicV1)], magicV1):
		version = 1
	case bytes.Equal(data[:len(magicV2)], magicV2):
		version = 2
	default:
		return nil, errors.New("arc: not a supported ARC archive")
	}

	entryCount := binutil.ReadU32LE(data, 12)
	entrySize := entrySizeForVersion(version)
	baseOffset := int64(headerSize) + int64(entryCount)*int64(entrySize)
	if int64(len(data)) < baseOffset {
		return nil, errors.New("arc: truncated entry table")
	}

	entries := make([]Entry, 0, entryCount)
	for i := uint32(0); i < entryCount; i++ {
		off := int64(headerSize) + int64(i)*int64(entrySize)
		entry := data[off : off+int64(entrySize)]

		nameBytes := entry[:nameSizeForVersion(version)]
		name, err := decodeEntryName(nameBytes)
		if err != nil {
			return nil, fmt.Errorf("arc: entry %d: %w", i, err)
		}
		rel, err := sanitizeRelativePath(name)
		if err != nil {
			return nil, fmt.Errorf("arc: entry %d: %w", i, err)
		}

		offsetPos := offsetPosForVersion(version)
		sizePos := offsetPos + 4
		eOffset := binutil.ReadU32LE(entry, offsetPos)
		eSize := binutil.ReadU32LE(entry, sizePos)

		absOffset := baseOffset + int64(eOffset)
		if absOffset+int64(eSize) > int64(len(data)) {
			return nil, fmt.Errorf("arc: entry %d exceeds file bounds", i)
		}

		entries = append(entries, Entry{
			RelativePath: rel,
			Offset:       eOffset,
			Size:         eSize,
		})
	}

	return &ArchiveInfo{Version: version, Entries: entries}, nil
}

func entrySizeForVersion(version int) int {
	if version == 1 {
		return 32
	}
	return 128
}

func nameSizeForVersion(version int) int {
	return 16
}

func offsetPosForVersion(version int) int {
	if version == 1 {
		return 16
	}
	return 96
}

func decodeEntryName(b []byte) (string, error) {
	n := bytes.IndexByte(b, 0)
	if n == -1 {
		n = len(b)
	}
	if n == 0 {
		return "", errors.New("empty name")
	}
	decoded, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), b[:n])
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func sanitizeRelativePath(name string) (string, error) {
	name = filepath.FromSlash(name)
	if filepath.IsAbs(name) {
		return "", errors.New("absolute path not allowed")
	}

	parts := strings.Split(name, string(filepath.Separator))
	var clean []string
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return "", errors.New("path traversal not allowed")
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return "", errors.New("empty path")
	}
	return filepath.Join(clean...), nil
}
