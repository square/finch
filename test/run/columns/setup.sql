CREATE DATABASE IF NOT EXISTS finch;

USE finch;

DROP TABLE IF EXISTS coltest1, coltest2, coltest3;

CREATE TABLE coltest1 (
  a int unsigned not null auto_increment primary key,
  b int not null default 0
);

CREATE TABLE coltest2 (
  x int unsigned not null default 0,
  y varchar(10) not null default "",
  z int not null default 0,
  primary key (x)
);

CREATE TABLE coltest3 (
  x int unsigned not null default 0,
  y varchar(10) not null default "",
  primary key (x)
);
