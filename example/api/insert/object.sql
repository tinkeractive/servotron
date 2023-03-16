-- note the returning clause
-- returns fields corresponding to the associated select query
-- server maps the result to the select query params
-- the associated select query result becomes response body
with recordset as (
	select *
	from json_to_recordset($2) as x(
		bucket_id int,
		name varchar
	)
)
insert into object(bucket_id, name)
select *
from recordset
where $1=$1
returning row_to_json(object.*)
