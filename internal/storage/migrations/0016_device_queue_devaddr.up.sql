ALTER TABLE IF EXISTS device_queue
    ADD COLUMN IF NOT EXISTS dev_addr bytea;
