-- +migrate Up
alter table device_profile
    add column adr_algorithm_id varchar(100) default 'default' not null;

alter table device_profile
    alter column adr_algorithm_id drop default;

-- +migrate Down
alter table device_profile
    drop column adr_algorithm_id;
