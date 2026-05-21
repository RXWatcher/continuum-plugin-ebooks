package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AnnotationChange is one entry in the change log. hlc is the
// lexicographically-sortable string form of the originating HLC
// timestamp; replicas pull changes by passing a cursor and
// receiving every row with hlc > cursor.
type AnnotationChange struct {
	HLC          string
	UserID       string
	AnnotationID string
	Op           string // "upsert" | "delete"
	Payload      json.RawMessage
	OriginNode   string
	CreatedAt    time.Time
}

// AppendAnnotationChange records one mutation. Caller passes the
// HLC string; the store does not mint timestamps (it doesn't own
// the clock).
func (s *Store) AppendAnnotationChange(ctx context.Context, c AnnotationChange) error {
	if c.HLC == "" || c.UserID == "" || c.AnnotationID == "" || c.Op == "" {
		return errors.New("hlc, user_id, annotation_id, op required")
	}
	if len(c.Payload) == 0 {
		c.Payload = json.RawMessage("{}")
	}
	if c.OriginNode == "" {
		c.OriginNode = "unknown"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO annotation_change (hlc, user_id, annotation_id, op, payload, origin_node)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (hlc) DO NOTHING
	`, c.HLC, c.UserID, c.AnnotationID, c.Op, c.Payload, c.OriginNode)
	if err != nil {
		return fmt.Errorf("append annotation_change: %w", err)
	}
	return nil
}

// PullAnnotationChanges returns up to `limit` changes for `userID`
// with hlc > `since`. The empty since cursor returns the whole
// history (subject to limit). Caller passes the last seen hlc as
// the cursor on the next page request.
//
// Rows are returned in hlc ascending order so a client can apply
// them in causal order to produce the final state.
func (s *Store) PullAnnotationChanges(ctx context.Context, userID, since string, limit int) ([]AnnotationChange, error) {
	if userID == "" {
		return nil, errors.New("user_id required")
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT hlc, user_id, annotation_id, op, payload, origin_node, created_at
		FROM annotation_change
		WHERE user_id = $1 AND hlc > $2
		ORDER BY hlc
		LIMIT $3
	`, userID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("pull annotation_change: %w", err)
	}
	defer rows.Close()
	var out []AnnotationChange
	for rows.Next() {
		var c AnnotationChange
		if err := rows.Scan(&c.HLC, &c.UserID, &c.AnnotationID, &c.Op, &c.Payload,
			&c.OriginNode, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
