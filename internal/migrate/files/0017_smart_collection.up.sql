-- Rule-based Smart Collections for ebooks. Mirrors the audiobooks
-- plugin's migration 0015 — same shape so the host vocabulary is
-- consistent. The manual `collection` table from migration 0002
-- remains for hand-curated lists; this is a separate surface whose
-- membership is computed from a query_def JSON DSL.
CREATE TABLE IF NOT EXISTS smart_collection (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL,
  name        TEXT NOT NULL,
  description TEXT,
  color       TEXT,
  is_public   BOOLEAN NOT NULL DEFAULT FALSE,
  is_pinned   BOOLEAN NOT NULL DEFAULT FALSE,
  query_def   JSONB NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS smart_collection_user_idx
  ON smart_collection (user_id, is_pinned DESC, name);
CREATE INDEX IF NOT EXISTS smart_collection_public_idx
  ON smart_collection (is_public, name) WHERE is_public = TRUE;
