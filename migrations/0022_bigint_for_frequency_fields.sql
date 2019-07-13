-- +migrate Up
alter table device_profile
    alter column ping_slot_freq type bigint,
    alter column rx_freq_2 type bigint,
    alter column factory_preset_freqs type bigint[];

alter table multicast_group
    alter column frequency type bigint;

-- +migrate Down
alter table multicast_group
    alter column frequency type integer;

alter table device_profile
    alter column ping_slot_freq type integer,
    alter column rx_freq_2 type integer,
    alter column factory_preset_freqs type integer[];
