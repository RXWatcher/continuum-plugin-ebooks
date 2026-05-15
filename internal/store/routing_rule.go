package store

import (
	"context"
	"fmt"
)

type RequestRoutingRule struct {
	ID             int64  `json:"id"`
	MediaType      string `json:"media_type"`
	TargetPluginID string `json:"target_plugin_id"`
	FormatPref     string `json:"format_pref,omitempty"`
	AutoMonitor    bool   `json:"auto_monitor"`
	Enabled        bool   `json:"enabled"`
	SortOrder      int    `json:"sort_order"`
}

func (s *Store) ListRequestRoutingRules(ctx context.Context, enabledOnly bool) ([]RequestRoutingRule, error) {
	sql := `
		SELECT id, media_type, target_plugin_id, COALESCE(format_pref,''), auto_monitor, enabled, sort_order
		FROM request_routing_rule`
	if enabledOnly {
		sql += ` WHERE enabled = TRUE`
	}
	sql += ` ORDER BY sort_order ASC, id ASC`
	rows, err := s.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("list request routing rules: %w", err)
	}
	defer rows.Close()
	var out []RequestRoutingRule
	for rows.Next() {
		var rule RequestRoutingRule
		if err := rows.Scan(
			&rule.ID, &rule.MediaType, &rule.TargetPluginID, &rule.FormatPref,
			&rule.AutoMonitor, &rule.Enabled, &rule.SortOrder,
		); err != nil {
			return nil, fmt.Errorf("scan request routing rule: %w", err)
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (s *Store) ResolveRequestRoutingRule(ctx context.Context, mediaType string) (RequestRoutingRule, error) {
	if mediaType == "" {
		mediaType = "book"
	}
	mediaType = normalizeRoutingMediaType(mediaType)
	var rule RequestRoutingRule
	err := s.pool.QueryRow(ctx, `
		SELECT id, media_type, target_plugin_id, COALESCE(format_pref,''), auto_monitor, enabled, sort_order
		FROM request_routing_rule
		WHERE enabled = TRUE AND media_type = $1
		LIMIT 1
	`, mediaType).Scan(
		&rule.ID, &rule.MediaType, &rule.TargetPluginID, &rule.FormatPref,
		&rule.AutoMonitor, &rule.Enabled, &rule.SortOrder,
	)
	if err != nil {
		return RequestRoutingRule{}, err
	}
	return rule, nil
}

func (s *Store) ReplaceRequestRoutingRules(ctx context.Context, rules []RequestRoutingRule) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	keepIDs := make([]int64, 0, len(rules))
	for i, rule := range rules {
		if rule.MediaType == "" {
			return fmt.Errorf("routing rule %d: media type is required", i+1)
		}
		rule.MediaType = normalizeRoutingMediaType(rule.MediaType)
		if rule.TargetPluginID == "" {
			return fmt.Errorf("routing rule %q: provider is required", rule.MediaType)
		}
		if rule.ID > 0 {
			_, err = tx.Exec(ctx, `
				UPDATE request_routing_rule
				   SET media_type = $2,
				       target_plugin_id = $3,
				       format_pref = NULLIF($4,''),
				       auto_monitor = $5,
				       enabled = $6,
				       sort_order = $7,
				       updated_at = now()
				 WHERE id = $1
			`, rule.ID, rule.MediaType, rule.TargetPluginID, rule.FormatPref, rule.AutoMonitor, rule.Enabled, rule.SortOrder)
			keepIDs = append(keepIDs, rule.ID)
		} else {
			err = tx.QueryRow(ctx, `
				INSERT INTO request_routing_rule
					(media_type, target_plugin_id, format_pref, auto_monitor, enabled, sort_order)
				VALUES ($1, $2, NULLIF($3,''), $4, $5, $6)
				ON CONFLICT (media_type) DO UPDATE SET
					target_plugin_id = EXCLUDED.target_plugin_id,
					format_pref = EXCLUDED.format_pref,
					auto_monitor = EXCLUDED.auto_monitor,
					enabled = EXCLUDED.enabled,
					sort_order = EXCLUDED.sort_order,
					updated_at = now()
				RETURNING id
			`, rule.MediaType, rule.TargetPluginID, rule.FormatPref, rule.AutoMonitor, rule.Enabled, rule.SortOrder).Scan(&rule.ID)
			keepIDs = append(keepIDs, rule.ID)
		}
		if err != nil {
			return err
		}
	}
	if len(keepIDs) == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM request_routing_rule`)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM request_routing_rule WHERE NOT (id = ANY($1))`, keepIDs)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func normalizeRoutingMediaType(mediaType string) string {
	switch mediaType {
	case "comics":
		return "comic"
	case "documents":
		return "document"
	case "magazines":
		return "magazine"
	case "mangas":
		return "manga"
	case "":
		return "book"
	default:
		return mediaType
	}
}
