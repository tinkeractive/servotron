with delete_object as (
	update object
	set active=false
	where bucket_id=$1::int
)
update bucket
set active=false
where bucket_id=$1::int
