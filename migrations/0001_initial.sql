-- +migrate Up
create table gateway (
	mac bytea primary key,
	name varchar(100) unique not null,
	description text not null,
	created_at timestamp with time zone not null,
	updated_at timestamp with time zone not null,
	first_seen_at timestamp with time zone,
	last_seen_at timestamp with time zone,
	location point,
	altitude double precision
);

create index idx_gateway_name on gateway (name);
create index idx_gateway_created_at on gateway (created_at);
create index idx_gateway_updated_at on gateway (updated_at);
create index idx_gateway_first_seen_at on gateway (first_seen_at);
create index idx_gateway_last_seen_at on gateway (last_seen_at);


create table gateway_stats (
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

create index idx_gateway_stats_mac on gateway_stats (mac);
create index idx_gateway_stats_timestamp on gateway_stats ("timestamp");
create index idx_gateway_stats_interval on gateway_stats ("interval");

-- +migrate Down
drop index idx_gateway_stats_interval;
drop index idx_gateway_stats_timestamp;
drop index idx_gateway_stats_mac;
drop table gateway_stats;

drop index idx_gateway_name;
drop index idx_gateway_created_at;
drop index idx_gateway_updated_at;
drop index idx_gateway_first_seen_at;
drop index idx_gateway_last_seen_at;
drop table gateway;
