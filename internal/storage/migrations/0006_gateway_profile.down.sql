DROP INDEX IF EXISTS idx_gateway_gateway_profile_id;
ALTER TABLE IF EXISTS gateway
    DROP COLUMN IF EXISTS gateway_profile_id;

DROP INDEX IF EXISTS idx_gateway_profile_extra_channel_gw_profile_id;
DROP TABLE IF EXISTS gateway_profile_extra_channel;

DROP TABLE IF EXISTS gateway_profile;
