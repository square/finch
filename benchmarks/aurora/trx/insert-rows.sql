-- prepare
-- copies: $params.tables
-- rows: $params.rows
INSERT INTO sbtest/*!copy-number*/ VALUES /*!csv 1000 (NULL, @k, @c, @pad)*/
