-- 0005 — replace plaintext kobo transfer_code with bcrypt code_hash.
--
-- Schema-clean strategy: in-flight kobo transfer sessions break on upgrade.
-- This is intentional and documented. Sessions are bounded to 30 minutes and
-- the URL itself is the credential — once an operator deploys this migration,
-- any outstanding transfer URLs handed out by the previous binary become 404.
-- That's safer than carrying plaintext codes through the upgrade window.
--
-- Lookup pattern: serve-file scans pending/active sessions and bcrypt-compares
-- code_hash against the URL-supplied code. The pending/active index keeps this
-- bounded (typically 0-10 rows per host at any time).

DELETE FROM kobo_transfer_session WHERE status IN ('pending','active');

ALTER TABLE kobo_transfer_session DROP COLUMN transfer_code;
ALTER TABLE kobo_transfer_session ADD COLUMN code_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE kobo_transfer_session ALTER COLUMN code_hash DROP DEFAULT;
