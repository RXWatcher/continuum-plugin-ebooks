-- Per-user Hardcover.app GraphQL API token. Hardcover.app is a
-- reading-tracker service with a GraphQL API at
-- https://api.hardcover.app/v1/graphql. The token authorises
-- progress + status pushes from the plugin on behalf of the user.
-- Same storage shape as readwise_token.
CREATE TABLE IF NOT EXISTS hardcover_token (
  user_id    TEXT PRIMARY KEY,
  token      TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
