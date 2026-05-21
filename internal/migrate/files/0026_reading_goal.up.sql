-- Per-user yearly listening goals. One row per (user, year, kind);
-- the kind enum distinguishes book-count goals from hour-count
-- goals (some users prefer "12 books this year", others "100
-- hours"). Progress is computed read-side from existing data
-- (progress + reading_session) so the goal table is metadata-
-- only.
CREATE TABLE IF NOT EXISTS reading_goal (
  user_id    TEXT NOT NULL,
  year       INT NOT NULL,
  -- kind: 'books' (finished count) | 'hours' (seconds_played sum)
  kind       TEXT NOT NULL,
  target     INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, year, kind)
);
