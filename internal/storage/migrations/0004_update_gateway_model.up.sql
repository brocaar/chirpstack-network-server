update gateway set location = '(0,0)' where location is null;
update gateway set altitude = 0 where altitude is null;

alter table gateway
	alter column location set not null,
	alter column altitude set not null;