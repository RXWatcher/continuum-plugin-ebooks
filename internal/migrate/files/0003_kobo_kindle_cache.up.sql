CREATE TABLE kobo_transfer_session (
  id             TEXT PRIMARY KEY,
  user_id        TEXT NOT NULL,
  book_id        TEXT NOT NULL,
  format         TEXT NOT NULL,
  transfer_code  TEXT NOT NULL UNIQUE,
  source_path    TEXT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'pending',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at     TIMESTAMPTZ NOT NULL,
  completed_at   TIMESTAMPTZ
);
CREATE INDEX kobo_transfer_active_idx ON kobo_transfer_session (status, expires_at) WHERE status IN ('pending','active');

CREATE TABLE ebook_file_cache (
  id                TEXT PRIMARY KEY,
  cache_key         TEXT UNIQUE NOT NULL,
  book_id           TEXT NOT NULL,
  format            TEXT NOT NULL,
  mime_type         TEXT NOT NULL,
  content_length    BIGINT NOT NULL,
  status            TEXT NOT NULL DEFAULT 'pending',
  error_message     TEXT,
  relative_path     TEXT NOT NULL,
  bytes_on_disk     BIGINT NOT NULL DEFAULT 0,
  last_accessed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ebook_file_cache_status_accessed_idx ON ebook_file_cache (status, last_accessed_at);
CREATE INDEX ebook_file_cache_book_format_idx ON ebook_file_cache (book_id, format);

CREATE TABLE kindle_send_log (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL,
  book_id      TEXT NOT NULL,
  format       TEXT NOT NULL,
  to_address   TEXT NOT NULL,
  status       TEXT NOT NULL,
  error_text   TEXT,
  sent_at      TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX kindle_send_log_user_created_idx ON kindle_send_log (user_id, created_at DESC);
