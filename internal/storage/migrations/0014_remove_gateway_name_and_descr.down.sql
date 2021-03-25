alter table gateway
    add column name varchar(100),
    add column description text;

update
    gateway
set
    name = encode(mac, 'hex'),
    description = encode(mac, 'hex');

alter table gateway
    alter column name set not null,
    alter column description set not null;

alter table gateway add unique (name);
create index idx_gateway_name on gateway(name);