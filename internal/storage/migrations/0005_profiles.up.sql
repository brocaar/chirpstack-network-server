create table device_profile (
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,

    device_profile_id uuid primary key,
    supports_class_b boolean not null,
    class_b_timeout integer not null,
    ping_slot_period integer not null,
    ping_slot_dr integer not null,
    ping_slot_freq integer not null,
    supports_class_c boolean not null,
    class_c_timeout integer not null,
    mac_version varchar(10) not null,
    reg_params_revision varchar(10) not null,
    rx_delay_1 integer not null,
    rx_dr_offset_1 integer not null,
    rx_data_rate_2 integer not null,
    rx_freq_2 integer not null,
    factory_preset_freqs integer[],
    max_eirp integer not null,
    max_duty_cycle integer not null,
    supports_join boolean not null,
    rf_region varchar(20) not null,
    supports_32bit_fcnt boolean not null
);

create index idx_device_profile_created_at on device_profile(created_at);
create index idx_device_profile_updated_at on device_profile(updated_at);

create table service_profile (
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,

    service_profile_id uuid primary key,
    ul_rate integer not null,
    ul_bucket_size integer not null,
    ul_rate_policy char(4) not null,
    dl_rate integer not null,
    dl_bucket_size integer not null,
    dl_rate_policy char(4) not null,
    add_gw_metadata boolean not null,
    dev_status_req_freq integer not null,
    report_dev_status_battery boolean not null,
    report_dev_status_margin boolean not null,
    dr_min integer not null,
    dr_max integer not null,
    channel_mask bytea,
    pr_allowed boolean not null,
    hr_allowed boolean not null,
    ra_allowed boolean not null,
    nwk_geo_loc boolean not null,
    target_per integer not null,
    min_gw_diversity integer not null
);

create index idx_service_profile_created_at on service_profile(created_at);
create index idx_service_profile_updated_at on service_profile(updated_at);

create table routing_profile (
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,

    routing_profile_id uuid primary key,
    as_id varchar(255)
);

create index idx_routing_profile_created_at on routing_profile(created_at);
create index idx_routing_profile_updated_at on routing_profile(updated_at);

create table device (
    dev_eui bytea primary key,
    created_at timestamp with time zone not null,
    updated_at timestamp with time zone not null,
    device_profile_id uuid not null references device_profile on delete cascade,
    service_profile_id uuid not null references service_profile on delete cascade,
    routing_profile_id uuid not null references routing_profile on delete cascade 
);

create index idx_device_created_at on device(created_at);
create index idx_device_updated_at on device(updated_at);
create index idx_device_device_profile_id on device(device_profile_id);
create index idx_device_service_profile_id on device(service_profile_id);
create index idx_device_routing_profile_id on device(routing_profile_id);

create table device_activation (
    id bigserial primary key,
    created_at timestamp with time zone not null,
    dev_eui bytea not null references device on delete cascade,
    join_eui bytea not null,
    dev_addr bytea not null,
    nwk_s_key bytea not null,
    dev_nonce bytea not null
);

create index idx_device_activation_created_at on device_activation(created_at);
create index idx_device_activation_join_eui on device_activation(join_eui);
create index idx_device_activation_dev_eui on device_activation(dev_eui);
create index idx_device_activation_dev_nonce on device_activation(dev_nonce);
