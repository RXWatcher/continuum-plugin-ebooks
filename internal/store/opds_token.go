package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type OPDSToken struct {
	ID         string
	UserID     string
	JTI        string
	TokenHash  string
	Label      string
	LastUsedAt time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

func (s *Store) InsertOPDSToken(ctx context.Context, t OPDSToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO opds_token (id, user_id, jti, token_hash, label)
		VALUES ($1, $2, $3, $4, $5)
	`, t.ID, t.UserID, t.JTI, t.TokenHash, t.Label)
	if err != nil {
		return fmt.Errorf("insert opds_token: %w", err)
	}
	return nil
}

func (s *Store) GetOPDSTokenByJTI(ctx context.Context, jti string) (OPDSToken, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, jti, token_hash, label, last_used_at, created_at, revoked_at
		FROM opds_token WHERE jti = $1 AND revoked_at IS NULL
	`, jti)
	var t OPDSToken
	if err := row.Scan(&t.ID, &t.UserID, &t.JTI, &t.TokenHash, &t.Label,
		&t.LastUsedAt, &t.CreatedAt, &t.RevokedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OPDSToken{}, ErrNotFound
		}
		return OPDSToken{}, fmt.Errorf("get opds_token: %w", err)
	}
	return t, nil
}

func (s *Store) TouchOPDSToken(ctx context.Context, jti string) error {
	_, err := s.pool.Exec(ctx, `UPDATE opds_token SET last_used_at = now() WHERE jti = $1`, jti)
	return err
}

func (s *Store) RevokeOPDSToken(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE opds_token SET revoked_at = now() WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AdminRevokeOPDSToken(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE opds_token SET revoked_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("admin revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListOPDSTokensByUser(ctx context.Context, userID string) ([]OPDSToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, jti, token_hash, label, last_used_at, created_at, revoked_at
		FROM opds_token WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list opds_tokens: %w", err)
	}
	defer rows.Close()
	return scanOPDSTokens(rows)
}

func (s *Store) ListAllOPDSTokens(ctx context.Context) ([]OPDSToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, jti, token_hash, label, last_used_at, created_at, revoked_at
		FROM opds_token ORDER BY created_at DESC LIMIT 500
	`)
	if err != nil {
		return nil, fmt.Errorf("list all: %w", err)
	}
	defer rows.Close()
	return scanOPDSTokens(rows)
}

func (s *Store) DeleteOPDSTokensRevokedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM opds_token WHERE revoked_at IS NOT NULL AND revoked_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune: %w", err)
	}
	return tag.RowsAffected(), nil
}

func scanOPDSTokens(rows pgx.Rows) ([]OPDSToken, error) {
	var out []OPDSToken
	for rows.Next() {
		var t OPDSToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.JTI, &t.TokenHash, &t.Label,
			&t.LastUsedAt, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}
