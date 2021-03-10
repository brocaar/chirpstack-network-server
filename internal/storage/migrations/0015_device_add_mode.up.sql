ALTER TABLE IF EXISTS device
    ADD COLUMN IF NOT EXISTS mode char(1) not null default 'A';

alter table IF EXISTS device
    ALTER COLUMN mode drop default;

UPDATE device
	SET mode = 'B'
FROM
	device_profile dp
WHERE
	device.device_profile_id = dp.device_profile_id
	AND dp.supports_class_b;

UPDATE device
	SET mode = 'C'
FROM
	device_profile dp
WHERE
	device.device_profile_id = dp.device_profile_id
	AND dp.supports_class_c;

CREATE INDEX IF NOT EXISTS idx_device_mode ON device(mode);
