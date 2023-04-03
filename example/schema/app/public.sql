-- set local param for table definitions with defaults
set local app_user.id='';

create schema if not exists public;
create extension if not exists "citext";

-- define some tables for the toy example
create sequence if not exists app_user_seq;
create table if not exists app_user (
	app_user_id int primary key default nextval('app_user_seq'),
	email_address citext,
	name varchar(63),
	created_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	created_at timestamptz default now(),
	updated_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	updated_at timestamptz default now(),
	active boolean default true
);
create sequence if not exists app_role_seq;
create table if not exists app_role (
	app_role_id int primary key default nextval('app_role_seq'), 
	name varchar(31),
	created_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	created_at timestamptz default now(),
	updated_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	updated_at timestamptz default now(),
	active boolean default true
);
create table if not exists app_user_map_app_role (
	app_user_id int references app_user(app_user_id),
	app_role_id int references app_role(app_role_id),
	created_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	created_at timestamptz default now(),
	updated_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	updated_at timestamptz default now(),	primary key (app_user_id, app_role_id)
);
create sequence if not exists bucket_seq;
create table if not exists bucket (
	bucket_id int primary key default nextval('bucket_seq'),
	name varchar(255),
	created_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	created_at timestamptz default now(),
	updated_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	updated_at timestamptz default now(),
	active boolean default true
);
create table if not exists bucket_map_app_user (
	bucket_id int references bucket(bucket_id),
	app_user_id int references app_user(app_user_id),
	primary key (bucket_id, app_user_id)
);
create sequence if not exists object_seq;
create table object (
	object_id int primary key default nextval('object_seq'),
	bucket_id int references bucket(bucket_id),
	name varchar(255),
	created_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	created_at timestamptz default now(),
	updated_by int default coalesce(nullif(current_setting('app_user.id'), '')::int, 1),
	updated_at timestamptz default now(),
	active boolean default true
);
insert into app_user(name, email_address)
	values ('Alpha', 'user_a@app.com'), ('Beta', 'user_b@app.com');
insert into app_role(name)
	values ('Superuser'), ('Analyst'), ('Reporter');
insert into app_user_map_app_role
	values (1, 1), (2, 2);
insert into bucket(name)
	values ('bucket_a'), ('bucket_b');
insert into bucket_map_app_user
	values (1, 1), (2, 2);
insert into object(bucket_id, name)
	values (1, 'object_a'), (2, 'object_b');

-- create servotron user
create user servotron;
grant all on schema public to servotron;
grant all on all tables in schema public to servotron;
