// Package store wraps pgx for the ebooks portal. All 11 tables from spec
// Layer 3 have typed wrappers in this package.
package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

var ErrNotFound = errors.New("not found")

// ErrKosyncUsernameTaken is returned by UpsertKosyncUser when the requested
// kosync username already belongs to a different silo user. Without this
// guard a second user registering the same username would overwrite the
// first user's credential row and inherit their reading progress.
var ErrKosyncUsernameTaken = errors.New("kosync username already taken")
