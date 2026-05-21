-- Per-user notification preferences for the audiobooks plugin.
-- Each row is one (user, category) toggle. Categories are
-- enumerated server-side; new categories add without a migration
-- because storage is just (user, category) strings.
--
-- Default behaviour for an unset (user, category) pair is "on" —
-- listeners opt OUT of categories they don't want, not opt in,
-- so a new category becomes visible to existing users by default.
CREATE TABLE IF NOT EXISTS notification_pref (
  user_id    TEXT NOT NULL,
  category   TEXT NOT NULL,
  enabled    BOOLEAN NOT NULL DEFAULT TRUE,
  -- delivery: 'inapp' | 'email' | 'push'. Each delivery channel
  -- can be configured independently for the same category.
  delivery   TEXT NOT NULL DEFAULT 'inapp',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, category, delivery)
);
CREATE INDEX IF NOT EXISTS notification_pref_user_idx
  ON notification_pref (user_id);
