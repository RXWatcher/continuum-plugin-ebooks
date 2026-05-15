ALTER TABLE kosync_progress DROP CONSTRAINT kosync_progress_pkey;
-- Collapse multi-device rows back to one row per (user_id, document); keep
-- the most-recent timestamp. We can't recover the dropped rows; the down
-- migration only restores the schema shape.
DELETE FROM kosync_progress a
  USING kosync_progress b
  WHERE a.user_id = b.user_id AND a.document = b.document
    AND a.timestamp < b.timestamp;
ALTER TABLE kosync_progress ALTER COLUMN device_id DROP NOT NULL;
ALTER TABLE kosync_progress ALTER COLUMN device_id DROP DEFAULT;
ALTER TABLE kosync_progress ADD PRIMARY KEY (user_id, document);
