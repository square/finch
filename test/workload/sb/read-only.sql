
BEGIN

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id=@id

-- prepare defer
SELECT c FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV

-- prepare defer
SELECT SUM(k) FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV

-- prepare defer
SELECT c FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV ORDER BY c

-- prepare defer
SELECT DISTINCT c FROM sbtest1 WHERE id BETWEEN @id_100 AND @PREV ORDER BY c

COMMIT
