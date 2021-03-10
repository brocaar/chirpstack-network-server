ALTER TABLE IF EXISTS gateway
    ADD COLUMN IF NOT EXISTS name varchar(100),
    ADD COLUMN IF NOT EXISTS description text;

UPDATE
    gateway
SET
    name = encode(mac, 'hex'),
    description = encode(mac, 'hex');

ALTER TABLE IF EXISTS gateway
    ALTER COLUMN name set not null,
    ALTER COLUMN description set not null;

ALTER TABLE IF EXISTS gateway add unique (name);
CREATE INDEX IF NOT EXISTS idx_gateway_name ON gateway(name);
