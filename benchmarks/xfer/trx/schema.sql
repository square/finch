
CREATE TABLE customers (
  id		bigint         NOT NULL AUTO_INCREMENT,
  c_token	varbinary(255) NOT NULL,
  country	char(3)        NOT NULL,
  c1 		varchar(20)    DEFAULT NULL,
  c2 		varchar(50)    DEFAULT NULL,
  c3 		varchar(255)   DEFAULT NULL,
  b1 		tinyint        NOT NULL,
  created_at	timestamp      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at	timestamp      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY  (c_token),
  KEY         (country)
) ENGINE=InnoDB;

CREATE TABLE balances (
  id 		bigint         NOT NULL AUTO_INCREMENT,
  b_token	varbinary(255) NOT NULL,
  c_token 	varbinary(255) NOT NULL,
  version 	int            NOT NULL DEFAULT '0',
  cents 	bigint         NOT NULL,
  currency	varbinary(3)   NOT NULL,
  c1		varchar(50)    NOT NULL,
  c2		varchar(120)   DEFAULT NULL,
  b1 		tinyint        NOT NULL,
  created_at	timestamp      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at	timestamp      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY  (b_token),
  KEY         (c_token)
) ENGINE=InnoDB;

CREATE TABLE xfers (
  id		bigint       NOT NULL AUTO_INCREMENT,
  x_token	varchar(255) NOT NULL,
  cents 	bigint       NOT NULL,
  currency 	varbinary(3) NOT NULL,
  s_token 	varchar(255) NOT NULL,
  r_token 	varchar(255) NOT NULL,
  version       int unsigned NOT NULL DEFAULT '0',
  c1		varchar(50)           DEFAULT NULL,
  c2		varchar(255)          DEFAULT NULL,
  c3 		varchar(30)           DEFAULT NULL,
  t1 		timestamp        NULL DEFAULT NULL,
  t2 		timestamp        NULL DEFAULT NULL,
  t3 		timestamp        NULL DEFAULT NULL,
  b1 		tinyint      NOT NULL,
  b2 		tinyint      NOT NULL,
  created_at	timestamp    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at	timestamp    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY 	(id),
  UNIQUE KEY  	(x_token),
  KEY    	(s_token, t1),
  KEY  		(r_token, t1),
  KEY    	(created_at)
) ENGINE=InnoDB;
