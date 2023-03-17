select count(*)>0
from bucket_map_app_user
join app_user using(app_user_id)
where email_address=$1
	and bucket_id=cast($2::json->>'bucket_id' as int)
