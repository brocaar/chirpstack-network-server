create index idx_service_profile_updated_at on service_profile(updated_at);
create index idx_service_profile_created_at on service_profile(created_at);

create index idx_routing_profile_updated_at on routing_profile(updated_at);
create index idx_routing_profile_created_at on routing_profile(created_at);

create index idx_gateway_profile_updated_at on gateway_profile(updated_at);
create index idx_gateway_profile_created_at on gateway_profile(created_at);

create index idx_gateway_last_seen_at on gateway(last_seen_at);
create index idx_gateway_first_seen_at on gateway(first_seen_at);
create index idx_gateway_updated_at on gateway(updated_at);
create index idx_gateway_created_at on gateway(created_at);

create index idx_device_profile_updated_at on device_profile(updated_at);
create index idx_device_profile_created_at on device_profile(created_at);

create index idx_device_updated_at on device(updated_at);
create index idx_device_created_at on device(created_at);