alter table gateway_profile
    add column stats_interval bigint not null default 30000000000;

alter table gateway_profile
    alter column stats_interval drop default;