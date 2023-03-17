-- if auth query succeeds, the query proceeds
-- as with auth query, AppUserCookieName is always first arg
-- URL route args are passed next
-- finally query string args are passed as specified in routes
-- multi row requests should return json array via json_agg
select json_agg(r)
from (
	select *
	from object
	-- each param must be used and since auth was already performed
	-- the AppUserCookieName (first) arg can be suppressed
	-- hence, $1=$1
	where $1=$1
		and bucket_id=$2
		and ($3::boolean is null or active=$3)
	limit $4
	offset $5
) as r


