create table multicast_group (
    id uuid primary key,
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,
    mc_addr bytea,
    mc_nwk_s_key bytea,
    f_cnt int not null,
    group_type char not null,
    dr int not null,
    frequency int not null,
    ping_slot_period int not null,
    routing_profile_id uuid not null references routing_profile on delete cascade,
    service_profile_id uuid not null references service_profile on delete cascade
);

create index idx_multicast_group_routing_profile_id on multicast_group(routing_profile_id);
create index idx_multicast_group_service_profile_id on multicast_group(service_profile_id);

create table device_multicast_group (
    dev_eui bytea references device on delete cascade,
    multicast_group_id uuid references multicast_group on delete cascade,
    created_at timestamp with time zone not null,

    primary key(multicast_group_id, dev_eui)
);

create table multicast_queue (
    id bigserial primary key,
    created_at timestamp with time zone not null,
    schedule_at timestamp with time zone not null,
    emit_at_time_since_gps_epoch bigint,
    multicast_group_id uuid not null references multicast_group on delete cascade,
    gateway_id bytea not null references gateway on delete cascade,
    f_cnt int not null,
    f_port int not null,
    frm_payload bytea
);

create index idx_multicast_queue_schedule_at on multicast_queue(schedule_at);
create index idx_multicast_queue_multicast_group_id on multicast_queue(multicast_group_id);
create index idx_multicast_queue_emit_at_time_since_gps_epoch on multicast_queue(emit_at_time_since_gps_epoch);