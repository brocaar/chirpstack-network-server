-- +migrate Up
create table device_queue (
    id bigserial primary key,
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,
    dev_eui bytea references device on delete cascade,
    frm_payload bytea,
    f_cnt int not null,
    f_port int not null,
    confirmed boolean not null,
    emit_at timestamp with time zone,
    forwarded_at timestamp with time zone,
    retry_count int not null
);

create index idx_device_queue_dev_eui on device_queue(dev_eui);
create index idx_device_queue_confirmed on device_queue(confirmed);
create index idx_device_queue_emit_at on device_queue(emit_at);
create index idx_device_queue_forwarded_at on device_queue(forwarded_at);

-- +migrate Down
drop index idx_device_queue_forwarded_at;
drop index idx_device_queue_emit_at;
drop index idx_device_queue_confirmed;
drop index idx_device_queue_dev_eui;

drop table device_queue;