-- +migrate Up
alter table device
    add column skip_fcnt_check boolean not null default false;

-- +migrate Down
alter table device
    drop column skip_fcnt_check;
