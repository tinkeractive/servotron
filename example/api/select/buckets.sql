select json_agg(r)
from (
	select bucket.*
	from bucket
	join bucket_map_app_user using(bucket_id)
	join app_user using(app_user_id)
	where email_address=$1
		and bucket.active=coalesce($2::boolean, true)
) as r
