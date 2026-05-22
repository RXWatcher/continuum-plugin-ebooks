DROP INDEX IF EXISTS collection_user_pinned_idx;
CREATE INDEX collection_user_pinned_idx
  ON collection (user_id, is_pinned DESC, name);
DROP INDEX IF EXISTS smart_collection_user_idx;
CREATE INDEX smart_collection_user_idx
  ON smart_collection (user_id, is_pinned DESC, name);
ALTER TABLE collection DROP COLUMN IF EXISTS profile_id;
ALTER TABLE smart_collection DROP COLUMN IF EXISTS profile_id;
