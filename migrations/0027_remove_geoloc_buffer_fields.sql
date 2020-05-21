-- +migrate Up
alter table device_profile
    drop column geoloc_buffer_ttl,
    drop column geoloc_min_buffer_size;

-- +migrate Down
alter table device_profile
    add column geoloc_buffer_ttl integer not null default 0,
    add column geoloc_min_buffer_size integer not null default 0;

alter table device_profile
    alter column geoloc_buffer_ttl drop default,
    alter column geoloc_min_buffer_size drop default;
