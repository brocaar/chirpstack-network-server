ALTER TABLE IF EXISTS routing_profile
    DROP COLUMN IF EXISTS ca_cert,
    DROP COLUMN IF EXISTS tls_cert,
    DROP COLUMN IF EXISTS tls_key;
