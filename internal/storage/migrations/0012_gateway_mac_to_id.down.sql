DROP INDEX IF EXISTS idx_gateway_gateway_id;

ALTER TABLE IF EXISTS gateway
    RENAME COLUMN gateway_id TO mac;
