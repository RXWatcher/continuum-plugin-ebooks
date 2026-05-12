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

CREATE TABLE kosync_user (
  user_id              TEXT NOT NULL,
  kosync_username      TEXT PRIMARY KEY,
  kosync_password_hash TEXT NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE kosync_progress (
  user_id     TEXT NOT NULL,
  document    TEXT NOT NULL,
  progress    TEXT NOT NULL,
  percentage  REAL NOT NULL DEFAULT 0,
  device      TEXT,
  device_id   TEXT,
  timestamp   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, document)
);
CREATE INDEX kosync_progress_user_ts_idx ON kosync_progress (user_id, timestamp DESC);

CREATE TABLE kosync_book_link (
  document    TEXT NOT NULL,
  book_id     TEXT NOT NULL,
  format      TEXT NOT NULL,
  user_id     TEXT NOT NULL,
  linked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (document, user_id)
);
