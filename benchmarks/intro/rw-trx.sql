BEGIN

SELECT c FROM finch.t1 WHERE id = @id FOR UPDATE

UPDATE finch.t1 SET n = n + 1 WHERE id = @id

COMMIT
