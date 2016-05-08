-- +migrate Up
alter table node
	add column rx_delay int2 not null default 0;

alter table node
	alter column rx_delay drop default;

-- +migrate Down

alter table node
	drop column rx_delay;
