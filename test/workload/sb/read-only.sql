
BEGIN

-- prepare
-- copy: 10
SELECT c FROM sbtest1 WHERE id=@id

-- prepare
SELECT c FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV

-- prepare
SELECT SUM(k) FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV

-- prepare
SELECT c FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV ORDER BY c

-- prepare
SELECT DISTINCT c FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV ORDER BY c

COMMIT
