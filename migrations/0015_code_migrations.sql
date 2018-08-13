-- +migrate Up
create table code_migration (
    id text primary key,
    applied_at timestamp with time zone not null
);

-- +migrate Down
drop table code_migration;
