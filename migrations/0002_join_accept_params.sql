-- +migrate Up
alter table node
	add column rx_delay int2 not null default 0;

alter table node
	alter column rx_delay drop default;

create table channel_set (
	id bigserial primary key,
	name character varying (100) not null
);

create table channel (
	id bigserial primary key,
	channel_set_id bigint references channel_set on delete cascade not null,
	channel integer not null,
	frequency integer not null,
	check (channel >= 0 and frequency > 0),
	unique (channel_set_id, channel)
);

-- +migrate Down

drop table channel;

drop table channel_set;

alter table node
	drop column rx_delay;
