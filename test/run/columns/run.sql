
-- save-insert-id: @a
INSERT INTO coltest1 (a) VALUES (NULL)

INSERT INTO coltest2 (x, y) VALUES (@a, "0x75")

-- save-columns: @x, @y, _
SELECT x, y, z FROM coltest2 LIMIT 1

INSERT INTO coltest3 (x, y) VALUES (@x, @y)
