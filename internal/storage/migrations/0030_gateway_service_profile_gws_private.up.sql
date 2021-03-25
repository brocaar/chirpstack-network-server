alter table service_profile
    add column gws_private boolean not null default false;

alter table gateway
    add column service_profile_id uuid references service_profile on delete cascade;

create index idx_gateway_service_profile_id on gateway(service_profile_id);