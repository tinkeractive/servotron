select json_agg(r)
from (
	select bucket.*
	from bucket
	join bucket_map_app_user using(bucket_id)
	where bucket.active=coalesce($1::boolean, true)
		and app_user_id=current_setting('app_user.id')::int
) as r
