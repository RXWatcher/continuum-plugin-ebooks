DROP TABLE IF EXISTS request_routing_rule;

ALTER TABLE request
  DROP COLUMN IF EXISTS media_type;
