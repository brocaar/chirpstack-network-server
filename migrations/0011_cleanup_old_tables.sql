-- +migrate Up
alter table gateway
    drop column channel_configuration_id;

drop table extra_channel;

drop table channel_configuration;

drop table frame_log;

-- +migrate Down
create table frame_log (
	id bigserial primary key,
	created_at timestamp with time zone not null,
	dev_eui bytea not null,
	rx_info_set jsonb,
	tx_info jsonb,
	phy_payload bytea not null
);

create index idx_frame_log_dev_eui on frame_log (dev_eui);
create index idx_frame_log_created_at on frame_log (created_at);

create table channel_configuration (
	id bigserial primary key,
	name varchar(100) unique not null,
	created_at timestamp with time zone not null,
	updated_at timestamp with time zone not null,
	band varchar(20) not null,
	channels smallint[] not null
);

create index idx_channel_configuration_created_at on channel_configuration(created_at);
create index idx_channel_configuration_updated_at on channel_configuration(updated_at);
create index idx_channel_configuration_band on channel_configuration(band);

create table extra_channel (
	id bigserial primary key,
	channel_configuration_id bigint not null references channel_configuration on delete cascade,
	created_at timestamp with time zone not null,
	updated_at timestamp with time zone not null,
	modulation varchar(10) not null,
	frequency integer not null,
	bandwidth integer not null,
	bit_rate integer not null,
	spread_factors smallint[]
);

create index idx_extra_channel_channel_configuration_id on extra_channel(channel_configuration_id);
create index idx_extra_channel_created_at on extra_channel(created_at);
create index idx_extra_channel_updated_at on extra_channel(updated_at);

alter table gateway
	add column channel_configuration_id bigint references channel_configuration on delete set null;

create index idx_gateway_channel_configuration_id on gateway(channel_configuration_id);
