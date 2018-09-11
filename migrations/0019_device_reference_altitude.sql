-- +migrate Up
alter table device
    add column reference_altitude double precision not null default 0;

alter table device
    alter column reference_altitude drop default;

-- +migrate Down
alter table device
    drop column reference_altitude;
