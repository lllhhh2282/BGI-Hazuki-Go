// Package audio implements reading BGI "bw  " audio containers.
package audio

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

const audioMagic = "bw  "

// IsBgiAudioFile reports whether path points to a BGI audio container.
// The format stores a 4-byte little-endian header size at offset 0,
// followed by the 4-byte magic "bw  " at offset 4.
func IsBgiAudioFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return IsBgiAudioBytes(data), nil
}

// IsBgiAudioBytes reports whether data starts with a BGI "bw  " audio header.
func IsBgiAudioBytes(data []byte) bool {
	return len(data) >= 8 && string(data[4:8]) == audioMagic
}

// ExtractBgiAudioToOgg extracts the raw OGG payload from a BGI audio
// container and writes it to outputPath.
func ExtractBgiAudioToOgg(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("audio: failed to read %s: %w", inputPath, err)
	}
	return ExtractBgiAudioBytesToOgg(data, outputPath)
}

// ExtractBgiAudioBytesToOgg extracts the raw OGG payload from an in-memory
// BGI audio container and writes it to outputPath.
func ExtractBgiAudioBytesToOgg(data []byte, outputPath string) error {
	if len(data) < 12 {
		return fmt.Errorf("audio: data too small for BGI audio header")
	}
	if string(data[4:8]) != audioMagic {
		return fmt.Errorf("audio: not a BGI audio file")
	}

	headerSize := binary.LittleEndian.Uint32(data[0:4])
	payloadSize := binary.LittleEndian.Uint32(data[8:12])
	end := int64(headerSize) + int64(payloadSize)
	if end > int64(len(data)) {
		return fmt.Errorf("audio: payload exceeds data size")
	}
	payload := data[headerSize:end]

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("audio: failed to create output directory: %w", err)
	}
	if err := os.WriteFile(outputPath, payload, 0o644); err != nil {
		return fmt.Errorf("audio: failed to write %s: %w", outputPath, err)
	}
	return nil
}
