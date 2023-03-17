[![Go Reference](https://pkg.go.dev/badge/github.com/tinkeractive/servotron)](https://pkg.go.dev/github.com/tinkeractive/servotron)

# servotron

## haiku
a deliverance -\
an app server for postgres\
without ORM

## dependencies
go

## install
git clone\
go build\
go install

## configuration
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

## app user cookie name
cookie used to identify user for authorization\
if empty string, all cookies passed as json key, value pairs

## file servers
static content such as HTML

## template servers\
templates written in the go text/template style\
useful for server side includes\
useful for altering content based on user roles/permissions\
server passes result of app user query to template

## route types
type         HTTP             SQL
-----        ----             ---          
create       POST             INSERT
read         GET              SELECT
update       PUT              UPDATE
delete       DELETE           DELETE
transaction  POST PUT DELETE  TRANSACTION
service      *                null

service route type is proxied to service URL

# example

## prerequesites
psql -f example/schema/app/public.sql

## run
servotron --config example/config.json

## load routes
curl localhost:9000/routes -d @example/routes.json

TODO finish example app\
TODO example requests
