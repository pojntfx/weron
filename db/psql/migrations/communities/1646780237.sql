-- +migrate Up
create table communities (
    id text primary key not null,
    password text not null,
    clients integer not null,
    persistent boolean not null
);
-- +migrate Down
drop table communities;