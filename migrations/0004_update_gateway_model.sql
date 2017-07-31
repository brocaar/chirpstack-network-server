-- +migrate Up
update gateway set location = '(0,0)' where location is null;
update gateway set altitude = 0 where altitude is null;

alter table gateway
	alter column location set not null,
	alter column altitude set not null;

-- +migrate Down
alter table gateway
	alter column location drop not null,
	alter column altitude drop not null;
