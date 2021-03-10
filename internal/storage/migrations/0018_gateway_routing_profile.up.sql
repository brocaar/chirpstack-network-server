ALTER TABLE IF EXISTS gateway
    ADD COLUMN IF NOT EXISTS routing_profile_id uuid REFERENCES routing_profile ON DELETE CASCADE;

UPDATE gateway
    SET routing_profile_id = (SELECT routing_profile_id FROM routing_profile LIMIT 1);

ALTER TABLE IF EXISTS gateway
    ALTER COLUMN routing_profile_id set not null;

CREATE INDEX IF NOT EXISTS idx_gateway_routing_profile_id on gateway(routing_profile_id);
