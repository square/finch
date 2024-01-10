CREATE TABLE t (
  id                  BIGINT        NOT NULL,
  other_id            BIGINT        NOT NULL,
  covered_column      VARCHAR(50)   NOT NULL,
  non_covered_column  VARCHAR(50)   NOT NULL,
  PRIMARY KEY (id),
  INDEX index_other_id_covered_column (other_id, covered_column)
)
