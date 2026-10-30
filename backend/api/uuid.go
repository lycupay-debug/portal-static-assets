// uuid.go provides cryptographically secure UUIDv4 identifiers for persisted records and managed files.

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

func newUUID() (string, error) {
	return newUUIDFrom(rand.Reader)
}

func newUUIDFrom(source io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(source, b); err != nil {
		return "", fmt.Errorf("generate UUIDv4: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
