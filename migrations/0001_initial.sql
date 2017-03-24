-- +migrate Up
create table gateway (
	mac bytea primary key,
	name text not null,
	description text not null,
	created_at timestamp with time zone not null,
	updated_at timestamp with time zone not null,
	first_seen_at timestamp with time zone,
	last_seen_at timestamp with time zone,
	location point,
	altitude double precision
);

create index idx_gateway_created_at on gateway (created_at);
create index idx_gateway_updated_at on gateway (updated_at);
create index idx_gateway_first_seen_at on gateway (first_seen_at);
create index idx_gateway_last_seen_at on gateway (last_seen_at);


create table gateway_stat (
	id bigserial primary key,
	mac bytea not null references gateway on delete cascade,
	"timestamp" timestamp with time zone not null,
	"interval" varchar(10) not null,
	rx_packets_received int not null,
	rx_packets_received_ok int not null,
	tx_packets_received int not null,
	tx_packets_emitted int not null,

	unique (mac, "timestamp", "interval")
);

create index idx_gateway_stat_mac on gateway_stat (mac);
create index idx_gateway_stat_timestamp on gateway_stat ("timestamp");
create index idx_gateway_stat_interval on gateway_stat ("interval");

-- +migrate Down
drop index idx_gateway_stat_interval;
drop index idx_gateway_stat_timestamp;
drop index idx_gateway_stat_mac;
drop table gateway_stat;

drop index idx_gateway_created_at;
drop index idx_gateway_updated_at;
drop index idx_gateway_first_seen_at;
drop index idx_gateway_last_seen_at;
drop table gateway;
