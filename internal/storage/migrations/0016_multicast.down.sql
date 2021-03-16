drop index idx_multicast_queue_emit_at_time_since_gps_epoch;
drop index idx_multicast_queue_multicast_group_id;
drop index idx_multicast_queue_schedule_at;

drop table multicast_queue;

drop table device_multicast_group;

drop index idx_multicast_group_service_profile_id;
drop index idx_multicast_group_routing_profile_id;
drop table multicast_group;