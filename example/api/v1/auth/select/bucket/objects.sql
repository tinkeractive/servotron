-- if authorized return true else return false
-- all params must be specified
-- URL route args are passed
-- followed by query string args as specified in routes
select count(*)>0
from bucket_map_app_user
where app_user_id=current_setting('app_user.id')::int
	and bucket_id=$1
	and ($2::boolean is null or $2=$2)
	and ($3::int is null or $3=$3)
	and ($4::int is null or $4=$4)


