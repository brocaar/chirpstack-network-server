-- +migrate Up
create table gateway (
	mac bytea primary key,
	created_at timestamp with time zone not null,
	updated_at timestamp with time zone not null,
	location point not null,
	altitude integer not null,
	rx_packets_received bigint not null,
	rx_packets_received_ok bigint not null,
	tx_packets_received bigint not null,
	tx_packets_emitted bigint not null
);

create index idx_gateway_created_at on gateway (created_at);
create index idx_gateway_updated_at on gateway (updated_at);

-- +migrate Down
drop index idx_gateway_created_at;
drop index idx_gateway_updated_at;
drop table gateway;
