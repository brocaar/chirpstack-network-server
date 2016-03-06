create table application (
	app_eui bytea primary key,
	name character varying (100) not null
);

create table node (
	dev_eui bytea primary key,
	app_eui bytea references application not null,
	app_key bytea not null,
	used_dev_nonces bytea
);

create index node_app_eui on node (app_eui);
