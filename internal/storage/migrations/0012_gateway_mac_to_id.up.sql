ALTER TABLE IF EXISTS gateway
    RENAME COLUMN mac TO gateway_id;

CREATE INDEX IF NOT EXISTS idx_gateway_gateway_id on gateway (gateway_id);
