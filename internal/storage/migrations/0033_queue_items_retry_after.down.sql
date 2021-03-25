alter table multicast_queue
    drop column updated_at,
    drop column retry_after;

alter table device_queue
    drop column retry_after;
