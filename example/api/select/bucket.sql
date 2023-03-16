select row_to_json(r)
from (
	select *
	from bucket
	where $1=$1
		and bucket_id=$2::int
) as r
