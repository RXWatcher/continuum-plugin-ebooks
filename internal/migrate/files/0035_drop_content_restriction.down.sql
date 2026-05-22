-- Per-user content restrictions for family / child accounts on the
-- ebooks plugin. Mirrors the audiobooks plugin's 0017 migration
-- minus the narrators column (ebooks don't have narrators). Admin
-- writes one row per restricted user; the catalog handlers drop
-- matching items before they leave the plugin.
CREATE TABLE IF NOT EXISTS content_restriction (
  user_id            TEXT PRIMARY KEY,
  blocked_genres     TEXT[] NOT NULL DEFAULT '{}',
  blocked_tags       TEXT[] NOT NULL DEFAULT '{}',
  blocked_authors    TEXT[] NOT NULL DEFAULT '{}',
  blocked_libraries  BIGINT[] NOT NULL DEFAULT '{}',
  explicit_blocked   BOOLEAN NOT NULL DEFAULT FALSE,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
