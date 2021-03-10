ALTER TABLE IF EXISTS device
    ADD COLUMN IF NOT EXISTS reference_altitude double precision not null default 0;

ALTER TABLE IF EXISTS device
    ALTER COLUMN reference_altitude drop default;
