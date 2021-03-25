alter table routing_profile
    add column ca_cert text not null default '',
    add column tls_cert text not null default '',
    add column tls_key text not null default '';