with record as (
	select * from json_to_record($1) as x(
		bucket_id int,
		name varchar
	)
)
update bucket
set name=record.name,
	updated_by=current_setting('app_user.id')::int
from record
where bucket.bucket_id=record.bucket_id
returning row_to_json(bucket.*)
