package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

// Regression: the public KOReader /kosync/users/create path uses
// CreateKosyncUserStrict, which must NEVER overwrite an existing account's
// password (that was the account-takeover vector), and standalone accounts
// must get a unique per-username owner id (so reading progress, keyed by
// user_id, is isolated rather than colliding on a shared empty user_id).
func TestCreateKosyncUserStrict_NoOverwrite_And_Isolated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.CreateKosyncUserStrict(ctx, store.KosyncUser{
		UserID: "kosync:alice", KosyncUsername: "alice", KosyncPasswordHash: "HASH_A",
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// An attacker re-POSTs the same username with a new password: must be
	// rejected, and the stored hash must be UNCHANGED.
	if err := s.CreateKosyncUserStrict(ctx, store.KosyncUser{
		UserID: "kosync:alice", KosyncUsername: "alice", KosyncPasswordHash: "ATTACKER_HASH",
	}); !errors.Is(err, store.ErrKosyncUsernameTaken) {
		t.Fatalf("re-create should be ErrKosyncUsernameTaken, got %v", err)
	}
	got, err := s.GetKosyncUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if got.KosyncPasswordHash != "HASH_A" {
		t.Fatalf("password was overwritten (account takeover): %q", got.KosyncPasswordHash)
	}
	if got.UserID != "kosync:alice" {
		t.Fatalf("owner id = %q, want kosync:alice", got.UserID)
	}

	// A different standalone user gets a distinct owner id → progress
	// (keyed by user_id) cannot collide across users.
	if err := s.CreateKosyncUserStrict(ctx, store.KosyncUser{
		UserID: "kosync:bob", KosyncUsername: "bob", KosyncPasswordHash: "HASH_B",
	}); err != nil {
		t.Fatalf("create bob: %v", err)
	}
	bob, _ := s.GetKosyncUserByUsername(ctx, "bob")
	if bob.UserID == got.UserID {
		t.Fatalf("alice and bob share an owner id %q — progress would collide", bob.UserID)
	}

	// The authenticated path (UpsertKosyncUser, owner-scoped) still lets the
	// real owner rotate, but a different owner cannot hijack the username.
	if err := s.UpsertKosyncUser(ctx, store.KosyncUser{
		UserID: "continuum-99", KosyncUsername: "alice", KosyncPasswordHash: "HIJACK",
	}); !errors.Is(err, store.ErrKosyncUsernameTaken) {
		t.Fatalf("cross-owner upsert should be rejected, got %v", err)
	}
	again, _ := s.GetKosyncUserByUsername(ctx, "alice")
	if again.KosyncPasswordHash != "HASH_A" {
		t.Fatalf("cross-owner upsert overwrote the hash: %q", again.KosyncPasswordHash)
	}
}
