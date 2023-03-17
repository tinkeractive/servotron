select count(*)>0
from object
join bucket_map_app_user using(bucket_id)
join app_user using(app_user_id)
where email_address=$1
	and object_id=$2::int
