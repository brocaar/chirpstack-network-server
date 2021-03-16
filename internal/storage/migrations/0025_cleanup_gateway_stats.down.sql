create table gateway_stats (
	id bigserial primary key,
	gateway_id bytea not null references gateway on delete cascade,
	"timestamp" timestamp with time zone not null,
	"interval" varchar(10) not null,
	rx_packets_received int not null,
	rx_packets_received_ok int not null,
	tx_packets_received int not null,
	tx_packets_emitted int not null,

	unique (gateway_id, "timestamp", "interval")
);

create index idx_gateway_stats_gateway_id on gateway_stats (gateway_id);
create index idx_gateway_stats_timestamp on gateway_stats ("timestamp");
create index idx_gateway_stats_interval on gateway_stats ("interval");