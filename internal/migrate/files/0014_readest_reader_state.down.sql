DROP INDEX IF EXISTS annotation_user_book_active_idx;
DROP INDEX IF EXISTS reader_config_user_updated_idx;

ALTER TABLE annotation
  DROP CONSTRAINT IF EXISTS annotation_kind_check;

ALTER TABLE annotation
  ADD CONSTRAINT annotation_kind_check
  CHECK (kind IN ('highlight','note'));

ALTER TABLE annotation
  DROP COLUMN IF EXISTS metadata_json,
  DROP COLUMN IF EXISTS deleted_at,
  DROP COLUMN IF EXISTS style,
  DROP COLUMN IF EXISTS page,
  DROP COLUMN IF EXISTS xpointer1,
  DROP COLUMN IF EXISTS xpointer0,
  DROP COLUMN IF EXISTS readest_type;

DROP TABLE IF EXISTS reader_config;
