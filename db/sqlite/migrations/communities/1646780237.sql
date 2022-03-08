-- +migrate Up
create table communities (
    id text primary key not null,
    password text not null,
    persistent bool not null
);
-- +migrate Down
drop table communities;