-- +migrate Up
create table gateway (
	mac bytea primary key,
	created_at timestamp with time zone not null,
	updated_at timestamp with time zone not null,
	first_seen_at timestamp with time zone,
	last_seen_at timestamp with time zone,
	location point not null,
	altitude integer
);

create index idx_gateway_created_at on gateway (created_at);
create index idx_gateway_updated_at on gateway (updated_at);
create index idx_gateway_first_seen_at on gateway (first_seen_at);
create index idx_gateway_last_seen_at on gateway (last_seen_at);

-- +migrate Down
drop index idx_gateway_created_at;
drop index idx_gateway_updated_at;
drop index idx_gateway_first_seen_at;
drop index idx_gateway_last_seen_at;
drop table gateway;
