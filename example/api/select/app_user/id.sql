select app_user_id::text
from app_user
where email_address=current_setting('app_user.auth')::json->>'email_address'
