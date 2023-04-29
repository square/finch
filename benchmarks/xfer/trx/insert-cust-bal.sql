
BEGIN

-- prepare
-- rows: $params.customers
INSERT INTO customers VALUES (NULL, @c_token, 'USA', @c_c1, @c_c2, @c_c3, 1, NOW(), NOW())

-- prepare
INSERT INTO balances VALUES
  (NULL, @b_token, @c_token, 1, 100, 'USD', @b_c1, @b_c2, 1, NOW(), NOW()),
  (NULL, @b_token, @c_token, 1, 200, 'USD', @b_c1, @b_c2, 1, NOW(), NOW()),
  (NULL, @b_token, @c_token, 1, 300, 'USD', @b_c1, @b_c2, 1, NOW(), NOW())

COMMIT
