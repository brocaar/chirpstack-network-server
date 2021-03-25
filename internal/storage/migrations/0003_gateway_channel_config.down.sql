drop index idx_gateway_channel_configuration_id;

alter table gateway
	drop column channel_configuration_id;

drop index idx_extra_channel_updated_at;
drop index idx_extra_channel_created_at;
drop index idx_extra_channel_channel_configuration_id;
drop table extra_channel;

drop index idx_channel_configuration_band;
drop index idx_channel_configuration_updated_at;
drop index idx_channel_configuration_created_at;
drop table channel_configuration;