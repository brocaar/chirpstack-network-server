-- +migrate Up
alter table device
    add column is_disabled boolean not null default false;

alter table device
    alter column is_disabled drop default;

-- +migrate Down
alter table device
    drop column is_disabled; 
