alter index idx_gateway_stats_gateway_id rename to idx_gateway_stats_mac;

alter table gateway_stats
    rename column gateway_id to mac;

alter table gateway
    rename column gateway_id to mac;