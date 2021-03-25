create table gateway_board (
    id smallint not null,
    gateway_id bytea references gateway on delete cascade,
    fpga_id bytea,
    fine_timestamp_key bytea,
    primary key(gateway_id, id)
);