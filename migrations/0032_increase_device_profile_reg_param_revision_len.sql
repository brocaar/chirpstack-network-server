-- +migrate Up
alter table device_profile
    alter column reg_params_revision type varchar(20);

-- +migrate Down
alter table device_profile
    alter column reg_params_revision type varchar(10);
