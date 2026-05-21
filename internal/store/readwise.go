package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetReadwiseToken returns the user's stored Readwise API token, or
// ErrNotFound when none is set. Token is bearer-style and the user
// rotates it any time on readwise.io.
func (s *Store) GetReadwiseToken(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", errors.New("user_id required")
	}
	var tok string
	err := s.pool.QueryRow(ctx, `
		SELECT token FROM readwise_token WHERE user_id = $1
	`, userID).Scan(&tok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get readwise_token: %w", err)
	}
	return tok, nil
}

func (s *Store) SetReadwiseToken(ctx context.Context, userID, token string) error {
	if userID == "" || token == "" {
		return errors.New("user_id, token required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO readwise_token (user_id, token)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET
			token      = EXCLUDED.token,
			updated_at = now()
	`, userID, token)
	if err != nil {
		return fmt.Errorf("set readwise_token: %w", err)
	}
	return nil
}

func (s *Store) DeleteReadwiseToken(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("user_id required")
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM readwise_token WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete readwise_token: %w", err)
	}
	return nil
}
