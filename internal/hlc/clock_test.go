package hlc

import (
	"testing"
)

// TestClock_Now_Monotonic verifies that successive Now() calls
// produce strictly increasing timestamps even when called in a
// tight loop (same wall-ms multiple times).
func TestClock_Now_Monotonic(t *testing.T) {
	c := New("node-a")
	prev := c.Now()
	for i := 0; i < 10000; i++ {
		next := c.Now()
		if !prev.Less(next) {
			t.Fatalf("Now() not monotonic at i=%d: %s -> %s", i, prev, next)
		}
		prev = next
	}
}

// TestClock_Observe_AdvancesAheadOfPeer confirms that ingesting a
// peer's timestamp pushes our local clock to be strictly greater on
// the next Now().
func TestClock_Observe_AdvancesAheadOfPeer(t *testing.T) {
	c := New("node-a")
	peer := Timestamp{Wall: 10_000_000_000_000, Counter: 5, NodeID: "node-b"}
	c.Observe(peer)
	next := c.Now()
	if !peer.Less(next) {
		t.Errorf("Now() did not advance past peer: peer=%s next=%s", peer, next)
	}
}

// TestTimestamp_String_RoundTrip ensures Parse(String) returns the
// same Timestamp. Critical because wire-format peers send strings.
func TestTimestamp_String_RoundTrip(t *testing.T) {
	in := Timestamp{Wall: 1_700_000_000_000, Counter: 42, NodeID: "node-xyz"}
	got, err := Parse(in.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != in {
		t.Errorf("round-trip mismatch: in=%v out=%v", in, got)
	}
}

// TestTimestamp_Less_DeterministicAcrossNodes verifies the node-id
// tiebreak — two replicas computing the same (wall, counter) at the
// same instant agree on order via their stable node-ids.
func TestTimestamp_Less_DeterministicAcrossNodes(t *testing.T) {
	a := Timestamp{Wall: 100, Counter: 1, NodeID: "alpha"}
	b := Timestamp{Wall: 100, Counter: 1, NodeID: "beta"}
	if !a.Less(b) || b.Less(a) {
		t.Errorf("node-id tiebreak inconsistent: a<b=%v b<a=%v", a.Less(b), b.Less(a))
	}
}

// TestParse_RejectsMalformed pins the strict-parse contract: bad
// input is an error, not a default time-zero timestamp.
func TestParse_RejectsMalformed(t *testing.T) {
	cases := []string{"", "abc", "100:", "100:abc:node", "100:1"}
	for _, in := range cases {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should have failed", in)
		}
	}
}
