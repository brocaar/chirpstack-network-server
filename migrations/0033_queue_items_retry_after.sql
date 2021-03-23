-- +migrate Up
alter table device_queue
    add column retry_after timestamp with time zone null;

alter table multicast_queue
    add column updated_at timestamp with time zone null,
    add column retry_after timestamp with time zone null;

update multicast_queue
    set
        updated_at = created_at;

alter table multicast_queue
    alter column updated_at set not null;

-- +migrate Down
alter table multicast_queue
    drop column updated_at,
    drop column retry_after;

alter table device_queue
    drop column retry_after;
