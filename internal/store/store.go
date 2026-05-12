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
