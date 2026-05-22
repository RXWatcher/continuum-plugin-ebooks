-- Collections become per-profile. profile_id '' is the primary profile;
-- existing rows default to it, which matches pre-profile behaviour. Ownership
-- is the (user_id, profile_id) pair — '' is unique only within a user.
ALTER TABLE collection
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE smart_collection
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS collection_user_pinned_idx;
CREATE INDEX collection_user_pinned_idx
  ON collection (user_id, profile_id, is_pinned DESC, name);

DROP INDEX IF EXISTS smart_collection_user_idx;
CREATE INDEX smart_collection_user_idx
  ON smart_collection (user_id, profile_id, is_pinned DESC, name);
