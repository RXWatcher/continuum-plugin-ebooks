// Package koboref implements a small in-process refcount registry shared by
// the Kobo serve handler and the KoboSessionReaper. It exists in its own
// package so both the server (which holds refs during io.Copy) and the
// scheduler (which consults refs before unlinking) can import it without
// creating an import cycle.
//
// This is the Kobo-side analog of streaming.Manager's refcount map and uses
// the same shape — sessions with active readers are skipped by the reaper
// and reconsidered on the next tick.
package koboref

import "sync"

// Registry tracks active readers of Kobo transfer session source files.
// The zero value is NOT usable; call New().
type Registry struct {
	mu   sync.Mutex
	refs map[string]int // session.ID → active reader count
}

// New returns an empty Registry. Safe for concurrent use.
func New() *Registry {
	return &Registry{refs: make(map[string]int)}
}

// Acquire increments the reader count for id and returns a release closure.
// Callers MUST defer the release exactly once. The closure is idempotent;
// only the first invocation decrements.
func (r *Registry) Acquire(id string) (release func()) {
	r.mu.Lock()
	r.refs[id]++
	r.mu.Unlock()
	var released bool
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if released {
			return
		}
		released = true
		r.refs[id]--
		if r.refs[id] <= 0 {
			delete(r.refs, id)
		}
	}
}

// InUse reports whether id has any active readers.
func (r *Registry) InUse(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refs[id] > 0
}
