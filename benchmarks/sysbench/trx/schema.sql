
CREATE TABLE sbtest1 (
  id int NOT NULL AUTO_INCREMENT,
  k int NOT NULL DEFAULT '0',
  c char(120) NOT NULL DEFAULT '',
  pad char(60) NOT NULL DEFAULT '',
  PRIMARY KEY (id)
  /* Secondary index added after loading rows */
) ENGINE=InnoDB
