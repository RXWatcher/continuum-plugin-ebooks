CREATE TABLE collection (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  name          TEXT NOT NULL,
  color         TEXT,
  is_public     BOOL NOT NULL DEFAULT false,
  is_pinned     BOOL NOT NULL DEFAULT false,
  cover_book_id TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX collection_user_pinned_idx ON collection (user_id, is_pinned DESC, name);

CREATE TABLE collection_item (
  collection_id TEXT NOT NULL REFERENCES collection(id) ON DELETE CASCADE,
  book_id       TEXT NOT NULL,
  position      INT NOT NULL,
  added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (collection_id, book_id)
);
