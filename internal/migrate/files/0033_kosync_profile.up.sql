-- kosync becomes per-profile. Existing rows default to '' (primary profile).
ALTER TABLE kosync_user
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE kosync_progress
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE kosync_book_link
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';

-- Re-key progress and book-link on the profile so each profile keeps its own
-- reading position.
-- kosync_progress PK before this migration: (user_id, document, device_id) — set by 0006.
ALTER TABLE kosync_progress DROP CONSTRAINT kosync_progress_pkey;
ALTER TABLE kosync_progress
  ADD PRIMARY KEY (user_id, profile_id, document);
ALTER TABLE kosync_book_link DROP CONSTRAINT kosync_book_link_pkey;
ALTER TABLE kosync_book_link
  ADD PRIMARY KEY (document, user_id, profile_id);
