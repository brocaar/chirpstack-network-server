alter table device
    add column mode char(1) not null default 'A';

alter table device
    alter column mode drop default;

update device
	set mode = 'B'
from
	device_profile dp
where
	device.device_profile_id = dp.device_profile_id
	and dp.supports_class_b;

update device
	set mode = 'C'
from
	device_profile dp
where
	device.device_profile_id = dp.device_profile_id
	and dp.supports_class_c;

create index idx_device_mode on device(mode);