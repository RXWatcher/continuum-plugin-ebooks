CREATE TABLE opds_token (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  jti           TEXT UNIQUE NOT NULL,
  token_hash    TEXT NOT NULL,
  label         TEXT NOT NULL DEFAULT '',
  last_used_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at    TIMESTAMPTZ
);
CREATE INDEX opds_token_user_idx ON opds_token (user_id) WHERE revoked_at IS NULL;
