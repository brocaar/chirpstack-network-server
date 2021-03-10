ALTER TABLE IF EXISTS service_profile
    ADD COLUMN IF NOT EXISTS gws_private boolean not null default false;

ALTER TABLE IF EXISTS gateway
    ADD COLUMN IF NOT EXISTS service_profile_id uuid references service_profile on delete cascade;

CREATE INDEX IF NOT EXISTS idx_gateway_service_profile_id on gateway(service_profile_id);
