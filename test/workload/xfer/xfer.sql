
BEGIN

-- save-result: @from_cust:s, @country:s
SELECT c_token, country FROM customers WHERE id=@id -- 2

-- save-result: @from_bal:s
SELECT b_token FROM balances WHERE c_token=@from_cust LIMIT 1 -- 3

-- save-result: @to_cust:s
SELECT c_token FROM customers WHERE id BETWEEN @id_100 AND @PREV AND country=@country LIMIT 1 -- 4

-- save-result: @to_bal:s
SELECT b_token FROM balances WHERE c_token=@to_cust LIMIT 1 -- 5

-- save-insert-id: @xfer_id:n
INSERT INTO xfers VALUES (NULL, @x_token, 100, 'USD', @from_bal, @to_bal, 1, @c1, @c2, @c3, NOW(), NULL, NULL, 1, 0, NOW(), NOW()) -- 6

-- save-result: @from_id:n, _
SELECT id, version FROM balances WHERE b_token=@from_bal FOR UPDATE -- 7

UPDATE balances SET cents=cents-100, version=version+1 WHERE id=@from_id -- 8

-- save-result: @to_id:n, _
SELECT id, version FROM balances WHERE b_token=@to_bal FOR UPDATE -- 9

UPDATE balances SET cents=cents+100, version=version+1 WHERE id=@to_id -- 10

UPDATE xfers SET t2=NOW(), c3='DONE' WHERE id=@xfer_id -- 11

COMMIT
