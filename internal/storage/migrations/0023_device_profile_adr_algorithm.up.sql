ALTER TABLE IF EXISTS device_profile
    ADD COLUMN IF NOT EXISTS adr_algorithm_id varchar(100) default 'default' not null;

ALTER TABLE IF EXISTS device_profile
    alter column adr_algorithm_id drop default;
