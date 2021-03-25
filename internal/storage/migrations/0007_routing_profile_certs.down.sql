alter table routing_profile
    drop column ca_cert,
    drop column tls_cert,
    drop column tls_key;