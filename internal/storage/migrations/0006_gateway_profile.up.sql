CREATE TABLE IF NOT EXISTS gateway_profile (
    gateway_profile_id uuid primary key,
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,
    channels smallint[] not null
);

CREATE TABLE IF NOT EXISTS gateway_profile_extra_channel (
    id bigserial primary key,
    gateway_profile_id uuid not null references gateway_profile on delete cascade,
    modulation varchar(10) not null,
    frequency integer not null,
    bandwidth integer not null,
    bitrate integer not null,
    spreading_factors smallint[]
);

CREATE INDEX IF NOT EXISTS idx_gateway_profile_extra_channel_gw_profile_id on gateway_profile_extra_channel(gateway_profile_id);

ALTER TABLE IF EXISTS gateway
    ADD COLUMN IF NOT EXISTS gateway_profile_id uuid references gateway_profile;

CREATE INDEX IF NOT EXISTS idx_gateway_gateway_profile_id on gateway(gateway_profile_id);
