-- prepare
INSERT INTO t (id, other_id, covered_column, non_covered_column) VALUES /*!csv 5000 (@n(), @n % 1000, @n, @n)*/
