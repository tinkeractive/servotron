-- atomic requests should return json object via row_to_json
select row_to_json(r)
from (
	select *
	from object
	join bucket_map_app_user using(bucket_id)
	join auth using(bucket_id, app_user_id)
	where $1=email_address
		bucket=$2
) as r
