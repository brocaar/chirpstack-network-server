-- +migrate Up
alter table gateway
    drop column first_seen_at,
    drop column last_seen_at;

-- +migrate Down
alter table gateway
    add column first_seen_at timestamp with time zone null,
    add column last_seen_at timestamp with time zone null;
