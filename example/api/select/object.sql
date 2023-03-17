-- atomic requests should return json object via row_to_json
select row_to_json(r)
from (
	select *
	from object
	where $1=$1
		and object_id=$2::int
		and active
) as r
