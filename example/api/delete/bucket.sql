with delete_object as (
	update object
	set active=false
	where bucket_id=$2::int
)
update bucket
set active=false
where $1=$1
	and bucket_id=$2::int
