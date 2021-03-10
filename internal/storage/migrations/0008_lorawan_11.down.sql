DROP INDEX IF EXISTS idx_device_activation_nonce_lookup;
CREATE INDEX IF NOT EXISTS idx_device_activation_created_at ON device_activation(created_at);
CREATE INDEX IF NOT EXISTS idx_device_activation_join_eui ON device_activation(join_eui);
CREATE INDEX IF NOT EXISTS idx_device_activation_dev_nonce ON device_activation(dev_nonce);

ALTER TABLE IF EXISTS device_activation
    DROP COLUMN IF EXISTS s_nwk_s_int_key,
    DROP COLUMN IF EXISTS nwk_s_enc_key,
    DROP COLUMN IF EXISTS join_req_type;

ALTER TABLE IF EXISTS device_activation
    RENAME COLUMN f_nwk_s_int_key TO nwk_s_key;
