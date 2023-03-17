with record as (
	select * from json_to_record($2) as x(
		object_id int,
		name varchar
	)
)
update object
set name=record.name
from record
where $1=$1
	and object.object_id=record.object_id
returning row_to_json(object.*)
