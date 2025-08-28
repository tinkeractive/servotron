with resultset as (
	-- get the data set unmolested by limit/offset
	-- this can be done without substantial performance penalty
	-- limit/offset is always applied _after_ dataset is materialized
	select id, val
	from foo
	where id in (1,2)
),
record_count as (
	-- get the count of records for pagination
	select count(*) as n
	from resultset
)
select row_to_json(t)
from (
	select * from (
		-- aggregate the resultset as a json aggregate
		-- if resultset is empty then the value is null
		select json_agg(r) as result
		from (
			-- apply the limit and offset
			-- no need to rearticulate the columns
			select *
			from resultset
			limit 2 offset 0
		) as r
	) as s
	-- bring in the record count by cross join to the single json result
	cross join record_count
) as t
