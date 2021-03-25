alter table device
    add column is_disabled boolean not null default false;

alter table device
    alter column is_disabled drop default;