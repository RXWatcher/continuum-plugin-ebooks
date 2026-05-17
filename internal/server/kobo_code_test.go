package server

import (
	"strings"
	"testing"
)

const koboAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func TestRandCode_LengthAlphabetAndEntropy(t *testing.T) {
	const n = 10
	seen := map[string]struct{}{}
	for i := 0; i < 2000; i++ {
		c, err := randCode(n)
		if err != nil {
			t.Fatalf("randCode error: %v", err)
		}
		if len(c) != n {
			t.Fatalf("randCode(%d) length = %d (%q)", n, len(c), c)
		}
		for _, r := range c {
			if !strings.ContainsRune(koboAlphabet, r) {
				t.Fatalf("randCode produced out-of-alphabet rune %q in %q", r, c)
			}
		}
		seen[c] = struct{}{}
	}
	// 2000 draws from a ~8e14 space must essentially never collide; a tiny
	// keyspace (the old 4-char code) would collide heavily here.
	if len(seen) < 1999 {
		t.Fatalf("excessive collisions: %d unique of 2000 — keyspace too small", len(seen))
	}
}
