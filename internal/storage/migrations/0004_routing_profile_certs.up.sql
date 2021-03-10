ALTER TABLE IF EXISTS routing_profile
    ADD COLUMN IF NOT EXISTS ca_cert text not null default '',
    ADD COLUMN IF NOT EXISTS tls_cert text not null default '',
    ADD COLUMN IF NOT EXISTS tls_key text not null default '';
