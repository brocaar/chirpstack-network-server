ALTER TABLE IF EXISTS gateway_profile
    ADD COLUMN IF NOT EXISTS stats_interval bigint not null default 30000000000;

ALTER TABLE IF EXISTS gateway_profile
    ALTER COLUMN stats_interval drop default;
