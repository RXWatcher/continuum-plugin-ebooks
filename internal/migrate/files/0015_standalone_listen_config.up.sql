ALTER TABLE backend_config
  ADD COLUMN IF NOT EXISTS standalone_http_listen TEXT NOT NULL DEFAULT '';
