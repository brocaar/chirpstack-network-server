DROP INDEX IF EXISTS idx_gateway_routing_profile_id;

ALTER TABLE IF EXISTS gateway
    DROP COLUMN IF EXISTS routing_profile_id;
