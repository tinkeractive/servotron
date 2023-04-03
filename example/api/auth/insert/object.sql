-- convert incoming json to recordset
with recordset as (
	select *
	from json_to_recordset($1) as x(
		bucket_id int,
		name varchar
	)
)
select count(*)>0
from recordset
join bucket_map_app_user using(bucket_id)
where app_user_id=current_setting('app_user.id')::int
