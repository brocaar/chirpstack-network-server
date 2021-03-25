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