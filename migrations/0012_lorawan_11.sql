-- +migrate Up
alter table device_activation
    rename column nwk_s_key to f_nwk_s_int_key;

alter table device_activation
    add column s_nwk_s_int_key bytea,
    add column nwk_s_enc_key bytea,
    add column dev_nonce_new integer,
    add column join_req_type smallint not null default 255;

update device_activation
set
    s_nwk_s_int_key = f_nwk_s_int_key,
    nwk_s_enc_key = f_nwk_s_int_key,
    dev_nonce_new = ('x' || right(dev_nonce::text, 4))::bit(16)::int;

drop index idx_device_activation_dev_nonce;

alter table device_activation
    alter column s_nwk_s_int_key set not null,
    alter column nwk_s_enc_key set not null,
    alter column join_req_type drop default,
    drop column dev_nonce;

alter table device_activation
    rename column dev_nonce_new to dev_nonce;

alter table device_activation
    alter column dev_nonce set not null;

drop index idx_device_activation_created_at;
drop index idx_device_activation_join_eui;

create index idx_device_activation_nonce_lookup on device_activation (join_eui, dev_eui, join_req_type, dev_nonce);

-- +migrate Down
drop index idx_device_activation_nonce_lookup;
create index idx_device_activation_created_at on device_activation(created_at);
create index idx_device_activation_join_eui on device_activation(join_eui);
create index idx_device_activation_dev_nonce on device_activation(dev_nonce);

alter table device_activation
    drop column s_nwk_s_int_key,
    drop column nwk_s_enc_key,
    drop column join_req_type;

alter table device_activation
    rename column f_nwk_s_int_key to nwk_s_key;
