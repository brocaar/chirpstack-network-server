-- +migrate Up
alter table device_queue
    add column dev_addr bytea;

-- +migrate Down
alter table device_queue
    drop column dev_addr;
