// uuid_test.go verifies UUIDv4 formatting and entropy failure propagation.

package main

import (
	"bytes"
	"errors"
	"testing"
)

type failingUUIDReader struct{}

func (failingUUIDReader) Read([]byte) (int, error) {
	return 0, errors.New("test entropy source unavailable")
}

// randomUUID is a test-only helper; production code must propagate entropy failures.
func randomUUID() string {
	id, err := newUUID()
	if err != nil {
		panic(err)
	}
	return id
}

func TestNewUUIDProducesRFC4122Version4(t *testing.T) {
	id, err := newUUIDFrom(bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if !portalUUID.MatchString(id) || id[14] != '4' {
		t.Fatalf("UUID is not RFC 4122 version 4: %q", id)
	}
	switch id[19] {
	case '8', '9', 'a', 'b':
	default:
		t.Fatalf("UUID variant bits are invalid: %q", id)
	}
}

func TestNewUUIDPropagatesEntropyFailure(t *testing.T) {
	if id, err := newUUIDFrom(failingUUIDReader{}); err == nil || id != "" {
		t.Fatalf("newUUIDFrom returned id=%q err=%v; want an error and no identifier", id, err)
	}
}
