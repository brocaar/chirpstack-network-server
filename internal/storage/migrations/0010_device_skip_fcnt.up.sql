alter table device
    add column skip_fcnt_check boolean not null default false;