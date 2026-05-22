-- OPDS auth moves to the core ValidateProfileCredential RPC; the per-user
-- token table is no longer used.
DROP TABLE IF EXISTS opds_token;
