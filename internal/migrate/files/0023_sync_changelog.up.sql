-- Annotation change log for replica sync. Every annotation mutation
-- (create / update / delete) writes a row here with the originating
-- HLC timestamp; replicas pull changes since their last known
-- timestamp and merge using row-level LWW.
--
-- Tombstones: deletes emit a row with op='delete' rather than
-- removing earlier rows, so a peer that hasn't seen an annotation
-- yet can still observe its eventual deletion.
CREATE TABLE IF NOT EXISTS annotation_change (
  -- HLC string form, lexicographically sortable. Acts as the
  -- cursor for incremental pulls (WHERE hlc > $cursor).
  hlc          TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL,
  annotation_id TEXT NOT NULL,
  -- op: 'upsert' | 'delete'. upsert carries the full row payload;
  -- delete carries just the id.
  op           TEXT NOT NULL,
  -- payload is the full annotation JSON at the time of the change.
  -- Deletes leave it as '{}'.
  payload      JSONB NOT NULL DEFAULT '{}',
  -- origin_node is the replica that produced the change; useful for
  -- audit + debugging "why did this row keep flipping" issues.
  origin_node  TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Per-user pull index: user pulls "changes for me since cursor".
CREATE INDEX IF NOT EXISTS annotation_change_user_hlc_idx
  ON annotation_change (user_id, hlc);
