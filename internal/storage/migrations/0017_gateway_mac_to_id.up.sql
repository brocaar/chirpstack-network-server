alter table gateway
    rename column mac to gateway_id;

alter table gateway_stats
    rename column mac to gateway_id;

alter index idx_gateway_stats_mac rename to idx_gateway_stats_gateway_id;