-- +migrate Up
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

alter table node
	add column rx_delay int2 not null default 0,
	add column rx1_dr_offset int2 not null default 0,
	add column channel_list_id bigint references channel_list on delete set null;

alter table node
	alter column rx_delay drop default,
	alter column rx1_dr_offset drop default;

-- +migrate Down
alter table node
	drop column rx_delay,
	drop column rx1_dr_offset,
	drop column channel_list_id;

drop table channel;

drop table channel_list;
