alter table gateway
	alter column location drop not null,
	alter column altitude drop not null;