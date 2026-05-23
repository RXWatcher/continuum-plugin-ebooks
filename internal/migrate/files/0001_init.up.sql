CREATE TABLE backend_config (
  id                              INT PRIMARY KEY DEFAULT 1,
  target_backend_plugin_id        TEXT NOT NULL DEFAULT '',
  auto_approve_requests           BOOL NOT NULL DEFAULT false,
  default_streaming_mode          TEXT NOT NULL DEFAULT 'proxy',
  cache_dir                       TEXT,
  cache_max_size_gb               INT NOT NULL DEFAULT 10,
  cache_download_concurrency      INT NOT NULL DEFAULT 4,
  path_remappings                 JSONB NOT NULL DEFAULT '[]'::jsonb,
  kosync_secret                   BYTEA NOT NULL,
  opds_realm                      TEXT NOT NULL DEFAULT 'Silo Library',
  kindle_smtp_config              JSONB NOT NULL DEFAULT '{}'::jsonb,
  kepubify_path                   TEXT NOT NULL DEFAULT '/usr/local/bin/kepubify',
  updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT backend_config_singleton CHECK (id = 1)
);

CREATE TABLE user_data (
  user_id        TEXT NOT NULL,
  book_id        TEXT NOT NULL,
  last_cfi       TEXT,
  current_page   INT,
  read_progress  REAL NOT NULL DEFAULT 0,
  is_finished    BOOL NOT NULL DEFAULT false,
  is_favorite    BOOL NOT NULL DEFAULT false,
  rating         SMALLINT CHECK (rating IS NULL OR rating BETWEEN 1 AND 5),
  notes          TEXT NOT NULL DEFAULT '',
  last_read_at   TIMESTAMPTZ,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, book_id)
);
CREATE INDEX user_data_user_lastread_idx ON user_data (user_id, last_read_at DESC NULLS LAST);
CREATE INDEX user_data_user_favorite_idx ON user_data (user_id, is_favorite) WHERE is_favorite;

CREATE TABLE annotation (
  id             TEXT PRIMARY KEY,
  user_id        TEXT NOT NULL,
  book_id        TEXT NOT NULL,
  cfi_range      TEXT NOT NULL,
  kind           TEXT NOT NULL CHECK (kind IN ('highlight','note')),
  color          TEXT,
  selected_text  TEXT NOT NULL DEFAULT '',
  note_text      TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX annotation_user_book_idx ON annotation (user_id, book_id, created_at);

CREATE TABLE request (
  id                TEXT PRIMARY KEY,
  user_id           TEXT NOT NULL,
  title             TEXT NOT NULL,
  authors           TEXT[] NOT NULL DEFAULT '{}',
  isbn              TEXT,
  source_id         TEXT,
  format_pref       TEXT,
  status            TEXT NOT NULL DEFAULT 'pending',
  target_plugin_id  TEXT NOT NULL,
  external_id       TEXT,
  auto_monitor      BOOL NOT NULL DEFAULT false,
  denied_reason     TEXT,
  failure_reason    TEXT,
  fulfilled_book_id TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  fulfilled_at      TIMESTAMPTZ
);
CREATE INDEX request_status_created_idx ON request (status, created_at DESC);
CREATE INDEX request_user_created_idx ON request (user_id, created_at DESC);
