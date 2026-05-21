-- Per-user Readwise.io API token. Stored as plain text (Readwise
-- tokens are bearer-style and the user can rotate any time); the
-- column is nullable so listeners without a Readwise account have
-- no row pressure. Index isn't useful since lookups are always
-- by user_id PK.
CREATE TABLE IF NOT EXISTS readwise_token (
  user_id    TEXT PRIMARY KEY,
  token      TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
