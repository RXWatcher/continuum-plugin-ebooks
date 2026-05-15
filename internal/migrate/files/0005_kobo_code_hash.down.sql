ALTER TABLE kobo_transfer_session DROP COLUMN code_hash;
ALTER TABLE kobo_transfer_session ADD COLUMN transfer_code TEXT NOT NULL DEFAULT '';
ALTER TABLE kobo_transfer_session ALTER COLUMN transfer_code DROP DEFAULT;
CREATE UNIQUE INDEX IF NOT EXISTS kobo_transfer_session_code_uidx ON kobo_transfer_session (transfer_code);
