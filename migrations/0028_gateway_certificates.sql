-- +migrate Up
alter table gateway
    add column tls_cert bytea;

-- +migrate Down
alter table gateway
    drop column tls_cert;
