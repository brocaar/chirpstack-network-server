alter table gateway
    add column routing_profile_id uuid references routing_profile on delete cascade;

update gateway
    set routing_profile_id = (select routing_profile_id from routing_profile limit 1);

alter table gateway
    alter column routing_profile_id set not null;

create index idx_gateway_routing_profile_id on gateway(routing_profile_id);