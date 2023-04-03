select row_to_json(r)
from (
	select bucket.*
	from bucket
	where bucket_id=$1::int
		and active
) as r
