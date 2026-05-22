-- Restore kosync_progress PK to (user_id, document, device_id) — the pre-0033 state set by 0006.
ALTER TABLE kosync_progress DROP CONSTRAINT kosync_progress_pkey;
ALTER TABLE kosync_progress ADD PRIMARY KEY (user_id, document, device_id);
ALTER TABLE kosync_book_link DROP CONSTRAINT kosync_book_link_pkey;
ALTER TABLE kosync_book_link ADD PRIMARY KEY (document, user_id);
ALTER TABLE kosync_user DROP COLUMN IF EXISTS profile_id;
ALTER TABLE kosync_progress DROP COLUMN IF EXISTS profile_id;
ALTER TABLE kosync_book_link DROP COLUMN IF EXISTS profile_id;
