create table frame_log (
	id bigserial primary key,
	created_at timestamp with time zone not null,
	dev_eui bytea not null,
	rx_info_set jsonb,
	tx_info jsonb,
	phy_payload bytea not null
);

create index idx_frame_log_dev_eui on frame_log (dev_eui);
create index idx_frame_log_created_at on frame_log (created_at);