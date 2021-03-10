ALTER TABLE IF EXISTS gateway
    ADD COLUMN IF NOT EXISTS tls_cert bytea;
