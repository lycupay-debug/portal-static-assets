package main

import (
	"reflect"
	"testing"
)

func TestSortedKeysIsDeterministic(t *testing.T) {
	input := map[string]bool{"zeta": true, "alpha": true, "middle": true}
	want := []string{"alpha", "middle", "zeta"}
	if got := sortedKeys(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedKeys() = %v, want %v", got, want)
	}
}

func TestSortedKeysEmptyMapReturnsEmptySlice(t *testing.T) {
	got := sortedKeys(map[string]bool{})
	if got == nil || len(got) != 0 {
		t.Fatalf("sortedKeys(empty) = %#v, want non-nil empty slice", got)
	}
}
