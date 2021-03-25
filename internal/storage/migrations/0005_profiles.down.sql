drop index idx_device_activation_dev_nonce;
drop index idx_device_activation_join_eui;
drop index idx_device_activation_dev_eui;
drop index idx_device_activation_created_at;
drop table device_activation;

drop index idx_device_routing_profile_id;
drop index idx_device_service_profile_id;
drop index idx_device_device_profile_id;
drop table device;

drop index idx_device_profile_created_at;
drop index idx_device_profile_updated_at;
drop table device_profile;

drop index idx_service_profile_created_at;
drop index idx_service_profile_updated_at;
drop table service_profile;

drop index idx_routing_profile_created_at;
drop index idx_routing_profile_updated_at;
drop table routing_profile;
