select count(*)>0
from bucket_map_app_user
where app_user_id=current_setting('app_user.id')::int
	and bucket_id=cast($1::json->>'bucket_id' as int)
