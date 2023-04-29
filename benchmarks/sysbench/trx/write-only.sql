
BEGIN

UPDATE sbtest1 SET k=k+1 WHERE id=@id

UPDATE sbtest1 SET c=@c WHERE id=@id

DELETE FROM sbtest1 WHERE id=@del_id

INSERT INTO sbtest1 (id, k, c, pad) VALUES (@del_id, @k, @c, @pad)

COMMIT
