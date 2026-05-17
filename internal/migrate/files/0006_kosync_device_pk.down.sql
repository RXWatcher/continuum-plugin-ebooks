ALTER TABLE kosync_progress DROP CONSTRAINT kosync_progress_pkey;
-- Collapse multi-device rows back to one row per (user_id, document), keeping
-- the most-recent timestamp. The previous `timestamp < timestamp` delete left
-- BOTH rows when two devices shared an identical timestamp (now() is
-- transaction-time, so same-tx upserts tie), which then made the
-- ADD PRIMARY KEY below fail and leave the migration dirty. ctid is a
-- guaranteed-unique per-row tiebreak, so exactly one row survives per group.
-- We can't recover the dropped rows; the down migration only restores shape.
DELETE FROM kosync_progress
WHERE ctid NOT IN (
  SELECT DISTINCT ON (user_id, document) ctid
  FROM kosync_progress
  ORDER BY user_id, document, timestamp DESC, ctid DESC
);
ALTER TABLE kosync_progress ALTER COLUMN device_id DROP NOT NULL;
ALTER TABLE kosync_progress ALTER COLUMN device_id DROP DEFAULT;
ALTER TABLE kosync_progress ADD PRIMARY KEY (user_id, document);
