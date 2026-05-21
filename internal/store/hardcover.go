package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Hardcover.app token storage. Same shape as the readwise_token
// helpers — GET / SET / DELETE keyed on user_id PK.
func (s *Store) GetHardcoverToken(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", errors.New("user_id required")
	}
	var tok string
	err := s.pool.QueryRow(ctx, `
		SELECT token FROM hardcover_token WHERE user_id = $1
	`, userID).Scan(&tok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get hardcover_token: %w", err)
	}
	return tok, nil
}

func (s *Store) SetHardcoverToken(ctx context.Context, userID, token string) error {
	if userID == "" || token == "" {
		return errors.New("user_id, token required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO hardcover_token (user_id, token)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET
			token      = EXCLUDED.token,
			updated_at = now()
	`, userID, token)
	if err != nil {
		return fmt.Errorf("set hardcover_token: %w", err)
	}
	return nil
}

func (s *Store) DeleteHardcoverToken(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("user_id required")
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM hardcover_token WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete hardcover_token: %w", err)
	}
	return nil
}
