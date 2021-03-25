drop index idx_gateway_routing_profile_id;

alter table gateway
    drop column routing_profile_id;