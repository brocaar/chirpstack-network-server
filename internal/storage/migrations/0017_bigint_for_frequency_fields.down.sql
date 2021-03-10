ALTER TABLE IF EXISTS multicast_group
    ALTER COLUMN frequency type integer;

ALTER TABLE IF EXISTS device_profile
    ALTER COLUMN ping_slot_freq type integer,
    ALTER COLUMN rx_freq_2 type integer,
    ALTER COLUMN factory_preset_freqs type integer[];
