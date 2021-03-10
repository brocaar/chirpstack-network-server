ALTER TABLE IF EXISTS device_activation
    RENAME COLUMN nwk_s_key TO f_nwk_s_int_key;

ALTER TABLE IF EXISTS device_activation
    ADD COLUMN IF NOT EXISTS s_nwk_s_int_key bytea,
    ADD COLUMN IF NOT EXISTS nwk_s_enc_key bytea,
    ADD COLUMN IF NOT EXISTS dev_nonce_new integer,
    ADD COLUMN IF NOT EXISTS join_req_type smallint not null default 255;

UPDATE device_activation
SET
    s_nwk_s_int_key = f_nwk_s_int_key,
    nwk_s_enc_key = f_nwk_s_int_key,
    dev_nonce_new = ('x' || right(dev_nonce::text, 4))::bit(16)::int;

DROP INDEX IF EXISTS idx_device_activation_dev_nonce;

ALTER TABLE IF EXISTS device_activation
    ALTER COLUMN s_nwk_s_int_key set not null,
    ALTER COLUMN nwk_s_enc_key set not null,
    ALTER COLUMN join_req_type drop default,
    DROP COLUMN IF EXISTS dev_nonce;

ALTER TABLE IF EXISTS device_activation
    RENAME COLUMN dev_nonce_new TO dev_nonce;

ALTER TABLE IF EXISTS device_activation
    ALTER COLUMN dev_nonce set not null;

DROP INDEX IF EXISTS idx_device_activation_created_at;
DROP INDEX IF EXISTS idx_device_activation_join_eui;

CREATE INDEX IF NOT EXISTS idx_device_activation_nonce_lookup ON device_activation (join_eui, dev_eui, join_req_type, dev_nonce);
