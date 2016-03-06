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

create table node_abp (
	dev_addr bytea primary key,
	dev_eui bytea references node not null,
	app_s_key bytea not null,
	nwk_s_key bytea not null,
	fcnt_up integer not null,
	fcnt_down integer not null
);

create index node_app_eui on node (app_eui);
create index node_abp_dev_eui on node (dev_eui);
