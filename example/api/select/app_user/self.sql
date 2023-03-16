select row_to_json(r)
from (
	select app_user_id,
		app_user.name,
		app_user.email_address,
		app_role_id,
		app_role.name as app_role_name
	from app_user
	join app_user_map_app_role using(app_user_id)
	join app_role using(app_role_id)
	where email_address=$1
) as r
