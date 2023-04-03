with record as (
	select * from json_to_record($1) as x(
		object_id int,
		name varchar
	)
)
update object
set name=record.name,
	updated_by=current_setting('app_user.id')::int
from record
where object.object_id=record.object_id
returning row_to_json(object.*)
