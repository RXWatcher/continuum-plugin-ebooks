-- Per-user ereader device registry. ABS lets users register named
-- email destinations (Kindle / Kobo / Boox / generic) and one-click
-- "send to <device>" from any book detail page. We already have the
-- SMTP send-to-Kindle plumbing in internal/kindle; this table is the
-- destination list the user picks from.
CREATE TABLE IF NOT EXISTS ereader_device (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL,
  -- name is user-supplied: "My Kindle", "Office iPad", ...
  name        TEXT NOT NULL,
  email       TEXT NOT NULL,
  -- vendor is a free-form hint the SPA renders as an icon: kindle,
  -- kobo, boox, generic.
  vendor      TEXT NOT NULL DEFAULT 'generic',
  -- preferred_format lets the user pin (epub / mobi / azw3 / pdf)
  -- so the send route picks the right file when multiple formats
  -- exist for one book. Empty = first available.
  preferred_format TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ereader_device_user_idx
  ON ereader_device (user_id, LOWER(name));
