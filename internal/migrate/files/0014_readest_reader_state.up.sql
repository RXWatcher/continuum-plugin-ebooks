CREATE TABLE reader_config (
  user_id     TEXT NOT NULL,
  book_id     TEXT NOT NULL,
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, book_id)
);

ALTER TABLE annotation
  ADD COLUMN IF NOT EXISTS readest_type TEXT NOT NULL DEFAULT 'annotation',
  ADD COLUMN IF NOT EXISTS xpointer0 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS xpointer1 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS page INT,
  ADD COLUMN IF NOT EXISTS style TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE annotation
  DROP CONSTRAINT IF EXISTS annotation_kind_check;

ALTER TABLE annotation
  ADD CONSTRAINT annotation_kind_check
  CHECK (kind IN ('highlight','note','bookmark','excerpt','annotation'));

CREATE INDEX IF NOT EXISTS reader_config_user_updated_idx
  ON reader_config (user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS annotation_user_book_active_idx
  ON annotation (user_id, book_id, updated_at DESC)
  WHERE deleted_at IS NULL;
