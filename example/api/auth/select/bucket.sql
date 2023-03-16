-- if authorized return true else return false
-- AppUserCookieName is always first arg
-- URL route args are passed next
-- finally query string args are passed as specified in routes
select count(*)>0
from bucket_map_app_user
join app_user using(app_user_id)
where email_address=$1
	and bucket_id=$2::int
