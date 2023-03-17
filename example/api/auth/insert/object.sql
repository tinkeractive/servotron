-- convert incoming json to recordset
with recordset as (
	select *
	from json_to_recordset($2) as x(
		bucket_id int,
		name varchar
	)
)
select count(*)>0
from recordset
join bucket_map_app_user using(bucket_id)
join app_user using(app_user_id)
where email_address=$1
