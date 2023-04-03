with recordset as (
	select *
	from json_to_recordset($1) as x(
		name varchar
	)
),
insert_bucket as (
	insert into bucket(name)
	select name
	from recordset
	returning bucket.*
),
insert_bucket_map_app_user as (
	insert into bucket_map_app_user
	select bucket_id,
		current_setting('app_user.id')::int as app_user_id
	from insert_bucket
)
select row_to_json(r)
from (
	select *
	from insert_bucket
) as r
