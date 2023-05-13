
BEGIN

UPDATE sbtest@tableNo SET k=k+1 WHERE id=@id

UPDATE sbtest@tableNo SET c=@c WHERE id=@id

DELETE FROM sbtest@tableNo WHERE id=@del_id

INSERT INTO sbtest@tableNo (id, k, c, pad) VALUES (@del_id, @k, @c, @pad)

COMMIT
