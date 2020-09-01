-- +migrate Up
alter table gateway_profile
    add column stats_interval bigint not null default 30000000000;

alter table gateway_profile
    alter column stats_interval drop default;

-- +migrate Down
alter table gateway_profile
    drop column stats_interval;
