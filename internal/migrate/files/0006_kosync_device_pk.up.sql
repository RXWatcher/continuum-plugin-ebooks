-- 0006 — bind kosync_progress rows to (user_id, document, device_id).
--
-- Previously the PK was (user_id, document) and device_id was free-form: a
-- malicious client posting under user A could overwrite user A's progress
-- regardless of which device wrote it (and the upsert key did not honour
-- per-device progress at all). The new PK isolates progress per device so
-- last-writer-wins is scoped to one (user, document, device) tuple and never
-- crosses devices for the same document.
--
-- device_id is now NOT NULL with default '' so the legacy "no device" case
-- (KOReader without an installation ID) collapses into a single deterministic
-- bucket per (user_id, document) instead of NULL-fanout.

ALTER TABLE kosync_progress ALTER COLUMN device_id SET DEFAULT '';
UPDATE kosync_progress SET device_id = '' WHERE device_id IS NULL;
ALTER TABLE kosync_progress ALTER COLUMN device_id SET NOT NULL;

ALTER TABLE kosync_progress DROP CONSTRAINT kosync_progress_pkey;
ALTER TABLE kosync_progress ADD PRIMARY KEY (user_id, document, device_id);
