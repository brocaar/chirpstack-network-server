DROP INDEX IF EXISTS idx_gateway_service_profile_id;

ALTER TABLE IF EXISTS gateway
    DROP COLUMN IF EXISTS service_profile_id;

ALTER TABLE IF EXISTS service_profile
    DROP COLUMN IF EXISTS gws_private;
