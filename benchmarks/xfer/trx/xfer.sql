
BEGIN

-- save-result: @sender_token, @country
SELECT c_token, country FROM customers WHERE id=@sender_id -- 2

-- save-result: @sender_balance_token
SELECT b_token FROM balances WHERE c_token=@sender_token LIMIT 1 -- 3

-- save-result: @receiver_token
SELECT c_token FROM customers WHERE id BETWEEN @receiver_id AND @PREV AND country=@country LIMIT 1 -- 4

-- save-result: @receiver_balance_token
SELECT b_token FROM balances WHERE c_token=@receiver_token LIMIT 1 -- 5

-- save-insert-id: @xfer_id
INSERT INTO xfers VALUES (NULL, @x_token, 100, 'USD', @sender_balance_token, @receiver_balance_token, 1, @c1, @c2, @c3, NOW(), NULL, NULL, 1, 0, NOW(), NOW()) -- 6

-- save-result: @sender_balance_id, _
SELECT id, version FROM balances WHERE b_token=@sender_balance_token FOR UPDATE -- 7

UPDATE balances SET cents=cents-100, version=version+1 WHERE id=@sender_balance_id -- 8

-- save-result: @receiver_balance_id, _
SELECT id, version FROM balances WHERE b_token=@receiver_balance_token FOR UPDATE -- 9

UPDATE balances SET cents=cents+100, version=version+1 WHERE id=@receiver_balance_id -- 10

UPDATE xfers SET t2=NOW(), c3='DONE' WHERE id=@xfer_id -- 11

COMMIT
