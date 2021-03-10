CREATE TABLE IF NOT EXISTS code_migration (
    id text primary key,
    applied_at timestamp with time zone not null
);