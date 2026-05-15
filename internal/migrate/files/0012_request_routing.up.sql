ALTER TABLE request
  ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'book';

CREATE TABLE IF NOT EXISTS request_routing_rule (
  id                 BIGSERIAL PRIMARY KEY,
  media_type         TEXT    NOT NULL UNIQUE,
  target_plugin_id   TEXT    NOT NULL,
  format_pref        TEXT,
  auto_monitor       BOOLEAN NOT NULL DEFAULT FALSE,
  enabled            BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order         INTEGER NOT NULL DEFAULT 0,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS request_routing_rule_enabled_idx
  ON request_routing_rule (enabled, sort_order, media_type);
