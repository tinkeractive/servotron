select count(*)>0
from object
join bucket_map_app_user using(bucket_id)
where app_user_id=current_setting('app_user.id')::int
	and object_id=cast($1::json->>'object_id' as int)
