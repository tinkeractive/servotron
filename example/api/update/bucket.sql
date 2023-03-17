with record as (
	select * from json_to_record($2) as x(
		bucket_id int,
		name varchar
	)
)
update bucket
set name=record.name
from record
where $1=$1
	and bucket.bucket_id=record.bucket_id
returning row_to_json(bucket.*)
