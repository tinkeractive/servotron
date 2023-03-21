# servotron

## Haiku
a deliverance -\
an app server for postgres\
without ORM

## Dependencies
go

## Install
git clone\
go build\
go install

## Configuration
```json
{
	"SQLRoot":"~/path/to/api/queries/root/dir",
	"FileServers":{
		"/assets":"~/path/to/static/content/www/assets",
		"/lib":"~/path/to/static/content/www/lib"
	},
	"TemplateServers":{
		"/":"~/path/to/go/templates"
	},
	"AppUserQuery":"~/path/to/app/user/api/query.sql",
	"ListenPort":"80"
	"ManagementPort":"9000",
	"DBConnString":"postgresql://postgres@localhost:5432/postgres",
	"DBPoolSize":4,
	"DBNotifyChannels":["public_default"],
	"AppUserCookieName":"EmailAddress",
	"Debug":true
}
```

### App User Cookie Name
cookie used to identify user for authorization\
if empty string, all cookies passed as json key, value pairs

### File Servers
static content such as HTML

### Template Servers
templates written in the go text/template style\
useful for server side includes\
useful for altering content based on user roles/permissions\
server passes result of app user query to template

## Route Types
type|HTTP|SQL
----|----|---
create|POST|INSERT
read|GET|SELECT
update|PUT|UPDATE
delete|DELETE|DELETE
transaction|POST|PUT|DELETE|TRANSACTION
service|*|null

service route type is proxied to service URL

# Example

## Prerequesites
psql -f example/schema/app/public.sql

## Run
servotron --config example/config.json

## Load Routes
curl localhost:9000/routes -d @example/routes.json

TODO finish example app\
TODO example requests
