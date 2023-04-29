
-- save-result: @token:s _ _ _ _ _
SELECT c_token, country, c1, c2, c3, b1 FROM customers WHERE id=@id

SELECT b_token, cents, currency, c1, c2, b1, updated_at FROM balances WHERE c_token=@token
