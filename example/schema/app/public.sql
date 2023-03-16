-- define some tables for the toy example
create schema if not exists public;
create extension if not exists "citext";
create sequence if not exists app_role_seq;
create table if not exists app_role (
	app_role_id int primary key default nextval('app_role_seq'), 
	name varchar(31),
	active boolean default true
);
create sequence if not exists app_user_seq;
create table if not exists app_user (
	app_user_id int primary key default nextval('app_user_seq'),
	email_address citext,
	name varchar(63),
	active boolean default true
);
create table if not exists app_user_map_app_role (
	app_user_id int references app_user(app_user_id),
	app_role_id int references app_role(app_role_id),
	primary key (app_user_id, app_role_id)
);
create sequence if not exists bucket_seq;
create table if not exists bucket (
	bucket_id int primary key default nextval('bucket_seq'),
	name varchar(255),
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
	active boolean default true
);
insert into app_role(name)
	values ('Superuser'), ('Analyst'), ('Reporter');
insert into app_user(name, email_address)
	values ('Alpha', 'user_a@app.com'), ('Beta', 'user_b@app.com');
insert into app_user_map_app_role
	values (1, 1), (2, 2);
insert into bucket(name)
	values ('bucket_a'), ('bucket_b');
insert into bucket_map_app_user
	values (1, 1), (2, 2);
insert into object(bucket_id, name)
	values (1, 'object_a'), (2, 'object_b');
