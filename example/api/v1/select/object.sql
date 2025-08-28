-- atomic requests should return json object via row_to_json
select row_to_json(r)
from (
	select object.*
	from object
	where object_id=$1::int
		and active
) as r
