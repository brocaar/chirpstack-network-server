create table gateway_profile (
    gateway_profile_id uuid primary key,
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,
    channels smallint[] not null
);

create index idx_gateway_profile_created_at on gateway_profile(created_at);
create index idx_gateway_profile_updated_at on gateway_profile(updated_at);

create table gateway_profile_extra_channel (
    id bigserial primary key,
    gateway_profile_id uuid not null references gateway_profile on delete cascade,
    modulation varchar(10) not null,
    frequency integer not null,
    bandwidth integer not null,
    bitrate integer not null,
    spreading_factors smallint[]
);

create index idx_gateway_profile_extra_channel_gw_profile_id on gateway_profile_extra_channel(gateway_profile_id);

alter table gateway
    add column gateway_profile_id uuid references gateway_profile;

create index idx_gateway_gateway_profile_id on gateway(gateway_profile_id);