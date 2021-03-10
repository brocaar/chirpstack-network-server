DROP INDEX IF EXISTS idx_device_queue_emit_at;

ALTER TABLE IF EXISTS device_queue
    DROP COLUMN IF EXISTS emit_at,
    ADD COLUMN IF NOT EXISTS emit_at_time_since_gps_epoch bigint;

CREATE INDEX IF NOT EXISTS idx_device_queue_emit_at_time_since_gps_epoch on device_queue(emit_at_time_since_gps_epoch);
