ALTER TABLE IF EXISTS device
    ADD COLUMN IF NOT EXISTS is_disabled boolean not null default false;

ALTER TABLE IF EXISTS device
    alter column is_disabled drop default;
