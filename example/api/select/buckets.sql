select json_agg(r)
from (
	select *
	from bucket
	join bucket_map_app_user using(bucket_id)
	join app_user using(app_user_id)
	where email_address=$1
		and ($2::boolean is null or bucket.active=$2)
) as r
