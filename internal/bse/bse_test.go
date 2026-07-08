package bse

import (
	"bytes"
	"testing"
)

func TestIsBseContainer(t *testing.T) {
	if IsBseContainer([]byte("BSE 1.0")) {
		t.Fatal("too-small data should not be a BSE container")
	}
	hdr := make([]byte, minSize)
	copy(hdr, magic)
	if !IsBseContainer(hdr) {
		t.Fatal("valid-size BSE header should be recognised by magic")
	}
}

func TestBseRoundTrip(t *testing.T) {
	inner := make([]byte, 64)
	for i := range inner {
		inner[i] = byte(i)
	}

	const seed uint32 = 0x12345678
	wrapped, err := EncryptBse(inner, seed)
	if err != nil {
		t.Fatalf("EncryptBse failed: %v", err)
	}

	if !IsBseContainer(wrapped) {
		t.Fatal("encrypted container missing BSE magic")
	}

	decrypted, err := DecryptBse(wrapped)
	if err != nil {
		t.Fatalf("DecryptBse failed: %v", err)
	}

	if !bytes.Equal(decrypted, inner) {
		t.Fatalf("BSE round-trip mismatch: got %d bytes, want %d", len(decrypted), len(inner))
	}
}

func TestBseChecksumFailure(t *testing.T) {
	inner := make([]byte, 64)
	wrapped, err := EncryptBse(inner, 0)
	if err != nil {
		t.Fatalf("EncryptBse failed: %v", err)
	}
	// Corrupt one encrypted byte.
	wrapped[headerSize+5] ^= 0xFF

	if _, err := DecryptBse(wrapped); err == nil {
		t.Fatal("DecryptBse should fail when checksum does not match")
	}
}
