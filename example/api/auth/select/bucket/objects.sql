-- if authorized return true else return false
-- all params must be specified
-- AppUserCookieName is always first arg
-- URL route args are passed next
-- finally query string args are passed as specified in routes
select count(*)>0
from bucket_map_app_user
join app_user using(app_user_id)
where email_address=$1
	and bucket_id=$2
	and ($3::boolean is null or $3=$3)
	and ($4::int is null or $4=$4)
	and ($5::int is null or $5=$5)


