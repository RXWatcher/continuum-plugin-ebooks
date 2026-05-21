// Package hlc implements a Hybrid Logical Clock for cross-replica
// ordering. HLC combines a wall-clock timestamp with a logical
// counter so events have a total order that respects causality even
// when wall clocks drift between replicas.
//
// Wire format: "<wall-ms>:<counter>:<node-id>" where wall-ms is the
// Unix milliseconds of the event's wall component, counter is the
// monotonic per-clock-tick disambiguator, and node-id is the
// replica's stable identifier. Sortable as a string when the
// wall-ms is zero-padded to a fixed width — we pad to 13 digits
// (covers wall times up to year 5138).
//
// Reference: Kulkarni et al., "Logical Physical Clocks", 2014.
package hlc

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Clock is a thread-safe HLC. Construct one per process with the
// replica's stable node-id; share across all callers that need
// timestamps for synced rows.
type Clock struct {
	nodeID  string
	mu      sync.Mutex
	lastMs  int64
	counter uint32
}

// New creates a Clock with the given node identifier. nodeID is
// embedded in every timestamp and must be stable across restarts
// (typically the plugin install id).
func New(nodeID string) *Clock {
	return &Clock{nodeID: nodeID}
}

// Now returns a fresh timestamp at the current wall time. The
// counter increments when the wall clock is the same as the last
// tick — guarantees monotonicity even under sub-millisecond bursts.
func (c *Clock) Now() Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()
	wall := time.Now().UnixMilli()
	if wall <= c.lastMs {
		// Wall went backwards (NTP adjustment) or two events
		// landed in the same ms — keep lastMs, bump counter.
		c.counter++
	} else {
		c.lastMs = wall
		c.counter = 0
	}
	return Timestamp{Wall: c.lastMs, Counter: c.counter, NodeID: c.nodeID}
}

// Observe advances the clock to be strictly greater than `peer`.
// Called when receiving a remote timestamp from another replica —
// keeps the local clock ahead of any incoming HLC so the next
// local event will be ordered after the peer's.
func (c *Clock) Observe(peer Timestamp) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if peer.Wall > c.lastMs {
		c.lastMs = peer.Wall
		c.counter = peer.Counter + 1
	} else if peer.Wall == c.lastMs && peer.Counter >= c.counter {
		c.counter = peer.Counter + 1
	}
}

// Timestamp is one HLC tick. Wall is Unix ms, Counter disambiguates
// same-ms ticks, NodeID is the originating replica.
type Timestamp struct {
	Wall    int64  `json:"wall"`
	Counter uint32 `json:"counter"`
	NodeID  string `json:"node_id"`
}

// String returns the sortable wire form. Zero-pad the wall ms to
// 13 digits so string comparison matches numeric comparison up to
// year 5138. Counter zero-padded to 10 digits (uint32 max).
func (t Timestamp) String() string {
	return fmt.Sprintf("%013d:%010d:%s", t.Wall, t.Counter, t.NodeID)
}

// Less reports whether t orders before u. Compares wall first
// (causal happens-before), then counter, then node-id (deterministic
// tiebreak so two replicas don't disagree on order for the
// astronomically-rare same-wall-same-counter case).
func (t Timestamp) Less(u Timestamp) bool {
	if t.Wall != u.Wall {
		return t.Wall < u.Wall
	}
	if t.Counter != u.Counter {
		return t.Counter < u.Counter
	}
	return t.NodeID < u.NodeID
}

// Parse decodes the string form back into a Timestamp. Returns an
// error on malformed input — callers should reject malformed
// timestamps from peers rather than treating them as time-zero.
func Parse(s string) (Timestamp, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return Timestamp{}, fmt.Errorf("hlc: want <wall>:<counter>:<node>, got %q", s)
	}
	wall, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Timestamp{}, fmt.Errorf("hlc: wall: %w", err)
	}
	counter, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return Timestamp{}, fmt.Errorf("hlc: counter: %w", err)
	}
	if parts[2] == "" {
		return Timestamp{}, fmt.Errorf("hlc: node id required")
	}
	return Timestamp{Wall: wall, Counter: uint32(counter), NodeID: parts[2]}, nil
}
