select json_agg(r)
from (
	select bucket.*
	from bucket
	where bucket.active=coalesce($1::boolean, true)
) as r
