DROP INDEX IF EXISTS idx_multicast_queue_emit_at_time_since_gps_epoch;
DROP INDEX IF EXISTS idx_multicast_queue_multicast_group_id;
DROP INDEX IF EXISTS idx_multicast_queue_schedule_at;

DROP TABLE IF EXISTS multicast_queue;

DROP TABLE IF EXISTS device_multicast_group;

DROP INDEX IF EXISTS idx_multicast_group_service_profile_id;
DROP INDEX IF EXISTS idx_multicast_group_routing_profile_id;
DROP TABLE IF EXISTS multicast_group;
