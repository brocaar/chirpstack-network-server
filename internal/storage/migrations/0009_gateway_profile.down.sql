drop index idx_gateway_gateway_profile_id;
alter table gateway
    drop column gateway_profile_id;

drop index idx_gateway_profile_extra_channel_gw_profile_id;
drop table gateway_profile_extra_channel;

drop index idx_gateway_profile_updated_at;
drop index idx_gateway_profile_created_at;
drop table gateway_profile;