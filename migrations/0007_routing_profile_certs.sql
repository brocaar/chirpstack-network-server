-- +migrate Up
alter table routing_profile
    add column ca_cert text not null default '',
    add column tls_cert text not null default '',
    add column tls_key text not null default '';


-- +migrate Down
alter table routing_profile
    drop column ca_cert,
    drop column tls_cert,
    drop column tls_key;
