

-- save-columns: @c
select c from t1 where id=1


insert into t2 values (@c)
