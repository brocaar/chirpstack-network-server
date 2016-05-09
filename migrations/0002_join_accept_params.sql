-- +migrate Up
alter table node
	add column rx_delay int2 not null default 0;

alter table node
	alter column rx_delay drop default;

create table channel_list (
	id bigserial primary key,
	name character varying (100) not null
);

create table channel (
	id bigserial primary key,
	channel_list_id bigint references channel_list on delete cascade not null,
	channel integer not null,
	frequency integer not null,
	check (channel >= 0 and frequency > 0),
	unique (channel_list_id, channel)
);

-- +migrate Down

drop table channel;

drop table channel_list;

alter table node
	drop column rx_delay;
