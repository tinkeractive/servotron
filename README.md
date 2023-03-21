# servotron

## Haiku
A deliverance -\
an app server for Postgres\
without ORM.

## Why?
"...Object/Relational Mapping is the Vietnam of Computer Science."

source: https://www.odbms.org/wp-content/uploads/2013/11/031.01-Neward-The-Vietnam-of-Computer-Science-June-2006.pdf

## How?
SQL.\
Prepared statements provide authorization (row-level security) and API endpoints.\
User info from cookies is always passed as the first argument to all prepared statements.\
Authorization queries should return a boolean value indicating whether the request is authorized for the user.\
All API prepared statements should return JSON. This is easily accomplished via PostgreSQL's JSON functions.\
For GET and DELETE requests, arguments that are parsed from the route and query string are passed in the order they appear in the route specification.\
For POST and PUT requests, the request body JSON is passed as the second argument. The JSON string can be transformed into a record or recordset (for bulk inserts) via PostgreSQL's JSON functions. The returned data from the INSERT and UPDATE should contain the fields required of the associated SELECT for the resource.\
Service routes are proxied to associated service URLs.

## Flow Diagram

![Alt text](doc/img/ServotronFlow.png "ServotronFlow")

## Dependencies
go

## Install
```bash
git clone
go build
go install
```

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
### Root Directories
File paths specified with tilde will resolve to the user home dir.\
This can cause errors when running with `sudo`.

### App User Cookie Name
Cookie used to identify user for authorization.\
If empty string, then all cookies are passed as json key, value pairs.

### File Servers
Static content such as HTML.

### Template Servers
Templates written in the go text/template style.\
Useful for server side includes.\
Useful for altering content based on user roles/permissions.\
Server passes the result of app user query to the template.

### Management Port
For admin functionality such as route loading.

### Pool Size
If not specified, this defaults to the number of CPUs.

### Notify Channels
Likely to be removed.\
Intended to enqueue messages.\
This functionality can be achieved by writing specialized agent listeners.

### Debug
If true, server writes error message responses to client.

## Route Types
type|HTTP|SQL
----|----|---
create|POST|INSERT
read|GET|SELECT
update|PUT|UPDATE
delete|DELETE|DELETE
transaction|POST|PUT|DELETE|TRANSACTION
service|*|null

Service route type is proxied to the service URL.

# Example

## Prerequisites
```bash
psql -f example/schema/app/public.sql
```

## Run
```bash
servotron --config example/config.json`
```

## Load Routes
```bash
curl localhost:9000/routes -d @example/routes.json`
```

TODO finish example app\
TODO example requests
