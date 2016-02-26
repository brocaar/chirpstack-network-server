create table application (
	app_eui bytea primary key,
	name character varying (100)
);

create table node (
	dev_eui bytea primary key,
	app_eui bytea references application,
	app_key bytea,
	used_dev_nonces bytea[]
);

create table node_abp (
	dev_addr bytea primary key,
	dev_eui bytea references node,
	app_s_key bytea,
	nwk_s_key bytea,
	fcnt_up integer,
	fcnt_down integer
);

create index node_app_eui on node (app_eui);
create index node_abp_dev_eui on node (dev_eui);
