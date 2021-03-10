DROP INDEX IF EXISTS idx_device_queue_emit_at_time_since_gps_epoch;

ALTER TABLE IF EXISTS device_queue
    DROP COLUMN IF EXISTS emit_at_time_since_gps_epoch,
    ADD COLUMN IF NOT EXISTS emit_at timestamp with time zone;

CREATE INDEX IF NOT EXISTS idx_device_queue_emit_at on device_queue(emit_at);
