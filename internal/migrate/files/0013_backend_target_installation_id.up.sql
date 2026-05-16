ALTER TABLE backend_config
  ADD COLUMN IF NOT EXISTS target_backend_installation_id TEXT NOT NULL DEFAULT '';

UPDATE backend_config
SET target_backend_installation_id = target_backend_plugin_id
WHERE target_backend_installation_id = ''
  AND target_backend_plugin_id ~ '^[0-9]+$';
