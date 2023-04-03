-- if auth query succeeds, the query proceeds
-- as with auth query, AppUserCookieName is always first arg
-- URL route args are passed next
-- finally query string args are passed as specified in routes
-- multi row requests should return json array via json_agg
select json_agg(r)
from (
	select *
	from object
	where bucket_id=$1
		and ($2::boolean is null or active=$2)
	limit $3
	offset $4
) as r


