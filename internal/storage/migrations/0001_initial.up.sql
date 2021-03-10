CREATE TABLE IF NOT EXISTS gateway (
	mac bytea primary key,
	name varchar(100) unique not null,
	description text not null,
	created_at timestamp with time zone not null,
	updated_at timestamp with time zone not null,
	first_seen_at timestamp with time zone,
	last_seen_at timestamp with time zone,
	location point not null,
	altitude double precision not null
);

CREATE INDEX IF NOT EXISTS idx_gateway_name on gateway (name);
