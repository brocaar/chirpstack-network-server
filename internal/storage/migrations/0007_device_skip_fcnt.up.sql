ALTER TABLE IF EXISTS device
    ADD COLUMN IF NOT EXISTS skip_fcnt_check boolean not null default false;
