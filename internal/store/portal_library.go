package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type PortalLibrary struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	MediaType        string `json:"media_type"`
	BackendPluginID  string `json:"backend_plugin_id"`
	BackendLibraryID *int64 `json:"backend_library_id,omitempty"`
	Enabled          bool   `json:"enabled"`
	SortOrder        int    `json:"sort_order"`
}

func (s *Store) ListPortalLibraries(ctx context.Context, enabledOnly bool) ([]PortalLibrary, error) {
	sql := `
		SELECT id, name, media_type, backend_plugin_id, backend_library_id, enabled, sort_order
		FROM portal_library`
	if enabledOnly {
		sql += ` WHERE enabled = TRUE`
	}
	sql += ` ORDER BY sort_order ASC, id ASC`
	rows, err := s.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PortalLibrary
	for rows.Next() {
		var l PortalLibrary
		if err := rows.Scan(
			&l.ID, &l.Name, &l.MediaType, &l.BackendPluginID, &l.BackendLibraryID,
			&l.Enabled, &l.SortOrder,
		); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) GetPortalLibrary(ctx context.Context, id int64) (PortalLibrary, error) {
	var l PortalLibrary
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, media_type, backend_plugin_id, backend_library_id, enabled, sort_order
		FROM portal_library
		WHERE id = $1 AND enabled = TRUE
	`, id).Scan(&l.ID, &l.Name, &l.MediaType, &l.BackendPluginID, &l.BackendLibraryID, &l.Enabled, &l.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return PortalLibrary{}, ErrNotFound
	}
	return l, err
}

func (s *Store) DefaultPortalLibrary(ctx context.Context) (PortalLibrary, error) {
	var l PortalLibrary
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, media_type, backend_plugin_id, backend_library_id, enabled, sort_order
		FROM portal_library
		WHERE enabled = TRUE
		ORDER BY sort_order ASC, id ASC
		LIMIT 1
	`).Scan(&l.ID, &l.Name, &l.MediaType, &l.BackendPluginID, &l.BackendLibraryID, &l.Enabled, &l.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return PortalLibrary{}, ErrNotFound
	}
	return l, err
}

func (s *Store) ReplacePortalLibraries(ctx context.Context, libs []PortalLibrary) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	keepIDs := make([]int64, 0, len(libs))
	for i, lib := range libs {
		if lib.Name == "" {
			return fmt.Errorf("library %d: name is required", i+1)
		}
		if lib.BackendPluginID == "" {
			return fmt.Errorf("library %q: backend plugin is required", lib.Name)
		}
		if lib.MediaType == "" {
			lib.MediaType = "book"
		}
		if lib.ID > 0 {
			_, err = tx.Exec(ctx, `
				UPDATE portal_library
				   SET name = $2,
				       media_type = $3,
				       backend_plugin_id = $4,
				       backend_library_id = $5,
				       enabled = $6,
				       sort_order = $7,
				       updated_at = now()
				 WHERE id = $1
			`, lib.ID, lib.Name, lib.MediaType, lib.BackendPluginID, lib.BackendLibraryID, lib.Enabled, lib.SortOrder)
			keepIDs = append(keepIDs, lib.ID)
		} else {
			err = tx.QueryRow(ctx, `
				INSERT INTO portal_library
					(name, media_type, backend_plugin_id, backend_library_id, enabled, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id
			`, lib.Name, lib.MediaType, lib.BackendPluginID, lib.BackendLibraryID, lib.Enabled, lib.SortOrder).Scan(&lib.ID)
			keepIDs = append(keepIDs, lib.ID)
		}
		if err != nil {
			return err
		}
	}
	if len(keepIDs) == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM portal_library`)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM portal_library WHERE NOT (id = ANY($1))`, keepIDs)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
