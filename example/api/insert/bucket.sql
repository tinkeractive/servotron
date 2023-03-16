with recordset as (
	select *
	from json_to_recordset($2) as x(
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
	select bucket_id, app_user_id
	from insert_bucket, app_user
	where email_address=$1
)
select row_to_json(r)
from (
	select *
	from insert_bucket
) as r
