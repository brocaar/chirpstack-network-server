alter table multicast_group
    alter column frequency type integer;

alter table device_profile
    alter column ping_slot_freq type integer,
    alter column rx_freq_2 type integer,
    alter column factory_preset_freqs type integer[];