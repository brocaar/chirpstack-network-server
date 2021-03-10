CREATE TABLE IF NOT EXISTS device_queue (
    id bigserial primary key,
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,
    dev_eui bytea references device on delete cascade,
    frm_payload bytea,
    f_cnt int not null,
    f_port int not null,
    confirmed boolean not null,
    is_pending boolean not null,
    emit_at timestamp with time zone,
    timeout_after timestamp with time zone
);

CREATE INDEX IF NOT EXISTS idx_device_queue_dev_eui on device_queue(dev_eui);
CREATE INDEX IF NOT EXISTS idx_device_queue_confirmed on device_queue(confirmed);
CREATE INDEX IF NOT EXISTS idx_device_queue_emit_at on device_queue(emit_at);
CREATE INDEX IF NOT EXISTS idx_device_queue_timeout_after on device_queue(timeout_after);
