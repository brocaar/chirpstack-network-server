DROP INDEX IF EXISTS idx_gateway_name;

ALTER TABLE IF EXISTS gateway
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS description;
