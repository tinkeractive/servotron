-- if authorized return true else return false
-- URL route args are passed first
-- followed by query string args as specified in routes
select count(*)>0
from bucket_map_app_user
where app_user_id=current_setting('app_user.id')::int
	and bucket_id=$1::int
