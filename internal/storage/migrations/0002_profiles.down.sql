DROP INDEX IF EXISTS idx_device_activation_dev_nonce;
DROP INDEX IF EXISTS idx_device_activation_join_eui;
DROP INDEX IF EXISTS idx_device_activation_dev_eui;
DROP INDEX IF EXISTS idx_device_activation_created_at;
DROP TABLE IF EXISTS device_activation;

DROP INDEX IF EXISTS idx_device_routing_profile_id;
DROP INDEX IF EXISTS idx_device_service_profile_id;
DROP INDEX IF EXISTS idx_device_device_profile_id;
DROP TABLE IF EXISTS device;

DROP TABLE IF EXISTS device_profile;

DROP TABLE IF EXISTS service_profile;

DROP TABLE IF EXISTS routing_profile;
