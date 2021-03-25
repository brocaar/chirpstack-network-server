drop index idx_device_queue_emit_at;

alter table device_queue
    drop column emit_at,
    add column emit_at_time_since_gps_epoch bigint;

create index idx_device_queue_emit_at_time_since_gps_epoch on device_queue(emit_at_time_since_gps_epoch);