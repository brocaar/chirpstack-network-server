ALTER TABLE IF EXISTS device_profile
    ALTER COLUMN ping_slot_freq type bigint,
    ALTER COLUMN rx_freq_2 type bigint,
    ALTER COLUMN factory_preset_freqs type bigint[];

ALTER TABLE IF EXISTS multicast_group
    ALTER COLUMN frequency type bigint;
