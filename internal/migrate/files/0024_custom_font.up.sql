-- Custom fonts uploaded by users for their own reading. Stored as
-- bytea inline rather than a filesystem blob so backups + replicas
-- carry the fonts; ebook fonts top out around 500KB which is
-- comfortable for inline storage.
--
-- Multi-user: per-user table, each user sees only their own fonts
-- in the reader font picker.
CREATE TABLE IF NOT EXISTS custom_font (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL,
  -- name is the font-family the CSS @font-face exposes; the reader
  -- writes this into the body font-family rule when this font is
  -- selected.
  name        TEXT NOT NULL,
  -- mime is the upload's content-type, used to set the response
  -- header when the reader fetches the font. Whitelist enforced
  -- handler-side (font/ttf, font/woff, font/woff2, font/otf).
  mime        TEXT NOT NULL,
  size_bytes  INT NOT NULL DEFAULT 0,
  data        BYTEA NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS custom_font_user_idx
  ON custom_font (user_id, LOWER(name));
