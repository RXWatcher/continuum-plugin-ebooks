-- Drops tables + columns belonging to features cut from scope:
-- audit log, per-library settings, sync change-log (HLC),
-- annotation Notebook (the table-side bits — Notebook reuses the
-- annotation table). IF EXISTS so fresh installs that never had
-- these tables run this cleanly.
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS annotation_change;
ALTER TABLE portal_library DROP COLUMN IF EXISTS settings;
