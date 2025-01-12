// TODO MySQL support
// TODO ingest OpenAPI specs for routes
// TODO connection pools for multiple databases (reader, writer)
// TODO consider prepared stmt handling

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// globals for simplified access, route reloading
var CONFIG Config
var ROUTER mux.Router
var SERVER *http.Server

func main() {
	configFilePath := flag.String("config", "", "config file path")
	flag.Parse()
	configBytes, err := os.ReadFile(*configFilePath)
	if err != nil {
		log.Fatal(err)
	}
	CONFIG, err = ParseConfig(configBytes)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("config:", CONFIG.String())
	pgxpoolConfig, err := pgxpool.ParseConfig(CONFIG.DBConnString)
	if err != nil {
		log.Fatal(err)
	}
	pgxpoolConfig.MinConns = int32(CONFIG.DBPoolSize)
	pgxpoolConfig.MaxConns = int32(CONFIG.DBPoolSize)
	// TODO implement AfterConnect to check connections
	pool, err := pgxpool.NewWithConfig(context.Background(), pgxpoolConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	log.Println(
		"connected to database",
		pool.Config().ConnConfig.Database,
		"at host",
		pool.Config().ConnConfig.Host,
		"on port",
		pool.Config().ConnConfig.Port,
		"as user",
		pool.Config().ConnConfig.User)
	log.Println("db pool size:", pool.Config().MinConns)
	// management server listening for admin requests on management port
	mgmtRouter := mux.NewRouter()
	mgmtRouter.HandleFunc("/routes", LoadRoutesHandler(pool)).Methods("POST")
	mgmtServer := &http.Server{
		Handler: mgmtRouter,
		Addr:    ":" + CONFIG.ManagementPort,
	}
	log.Println("listening on management port", CONFIG.ManagementPort)
	go mgmtServer.ListenAndServe()
	// server
	SERVER = &http.Server{Addr: ":" + CONFIG.ListenPort}
	log.Println("listening on port", CONFIG.ListenPort)
	log.Fatal(SERVER.ListenAndServe())
}

type Config struct {
	// file
	Debug              bool
	ListenPort         string
	ManagementPort     string
	DBConnString       string
	DBPoolSize         int
	DBNotifyChannels   []string
	AppUserAuth        map[string]string
	AppUserLocalParams map[string]string
	SQLRoot            string
	FileServers        map[string]string
	TemplateServers    map[string]string
	QueryStringAsJSON  bool
	// runtime
	QueryParams map[string][]string
	Routes      []Route
}

// NOTE if route json changes, then the route struct must change
type Route struct {
	Name        string
	Type        string
	URLScheme   string
	QueryParams []string
	ServiceURL  string
	Description string
}

func (c *Config) String() string {
	return fmt.Sprintf("%+v", *c)
}

func ParseConfig(b []byte) (Config, error) {
	var c Config
	c.Debug = false
	c.ListenPort = "80"
	c.DBConnString = "postgresql://postgres@localhost:5432/postgres"
	c.DBPoolSize = runtime.NumCPU()
	c.DBNotifyChannels = []string{"public_default"}
	c.AppUserAuth = make(map[string]string)
	c.AppUserAuth["Claim"] = ""
	c.AppUserAuth["Name"] = ""
	c.AppUserLocalParams = make(map[string]string)
	c.QueryStringAsJSON = true
	err := json.Unmarshal(b, &c)
	if err != nil {
		return c, err
	}
	who, err := user.Current()
	if err != nil {
		return c, err
	}
	c.SQLRoot, err = ResolveUserDir(who.HomeDir, c.SQLRoot)
	if err != nil {
		return c, err
	}
	for key, val := range c.FileServers {
		c.FileServers[key], err = ResolveUserDir(who.HomeDir, val)
		if err != nil {
			return c, err
		}
	}
	for key, val := range c.TemplateServers {
		c.TemplateServers[key], err = ResolveUserDir(who.HomeDir, val)
		if err != nil {
			return c, err
		}
	}
	for key, val := range c.AppUserLocalParams {
		c.AppUserLocalParams[key], err = ResolveUserDir(who.HomeDir, val)
		if err != nil {
			return c, err
		}
	}
	return c, err
}

func ResolveUserDir(homeDir, path string) (string, error) {
	result := path
	if strings.HasPrefix(path, "~/") {
		result = filepath.Join(homeDir, path[2:])
	}
	return filepath.Abs(result)
}

func LoadRoutesHandler(pool *pgxpool.Pool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("loading routes")
		bytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Println(FormatErr(err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(FormatErr(err.Error()))
			return
		}
		routes, err := GetRoutesFromBytes(bytes)
		err = LoadRouter(pool, routes)
		if err != nil {
			log.Println(FormatErr(err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(FormatErr(err.Error()))
			return
		}
		for _, route := range CONFIG.Routes {
			log.Println(route)
		}
		j, err := json.Marshal(CONFIG.Routes)
		if err != nil {
			log.Println(FormatErr(err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(FormatErr(err.Error()))
			return
		}
		w.Write(j)
	}
}

// create new router via side effects applied to server and router globals
func LoadRouter(pool *pgxpool.Pool, routes []Route) error {
	router, err := CreateRouter(pool, routes)
	if err != nil {
		log.Println(FormatErr(err.Error()))
		return err
	}
	ROUTER = *router
	SERVER.Handler = router
	return err
}

func CreateRouter(pool *pgxpool.Pool, routes []Route) (*mux.Router, error) {
	router := mux.NewRouter()
	CONFIG.Routes = routes
	err := LoadRoutes(router, pool, CONFIG.Routes)
	if err != nil {
		return router, err
	}
	for endpoint, dir := range CONFIG.FileServers {
		router.PathPrefix(endpoint).Handler(http.FileServer(http.Dir(dir)))
	}
	for endpoint, dir := range CONFIG.TemplateServers {
		router.PathPrefix(endpoint).Name(endpoint).HandlerFunc(HandleTemplateReq(pool, dir))
	}
	return router, nil
}

func GetRoutesFromBytes(bytes []byte) ([]Route, error) {
	var result []Route
	var err error
	if 0 < len(bytes) {
		err = json.Unmarshal(bytes, &result)
	}
	return result, err
}

func GetRoutesFromFile(path string) ([]Route, error) {
	var result []Route
	bytes, err := os.ReadFile(path)
	if 0 < len(bytes) {
		err = json.Unmarshal(bytes, &result)
	}
	return result, err
}

func LoadRoutes(router *mux.Router, pool *pgxpool.Pool, routes []Route) error {
	err := error(nil)
	CONFIG.QueryParams = make(map[string][]string)
	for _, r := range routes {
		switch r.Type {
		case "service":
			serviceURL, err := url.Parse(r.ServiceURL)
			if err != nil {
				return err
			}
			// TODO comment
			serviceProxy := httputil.NewSingleHostReverseProxy(serviceURL)
			serviceAuthFunc := AuthorizeReq(pool, serviceProxy.ServeHTTP)
			serviceFunc := CreateServiceFunc(r.URLScheme, serviceAuthFunc)
			router.PathPrefix(r.URLScheme).
				HandlerFunc(serviceFunc).
				Name(r.Name).
				Methods("GET", "POST", "PUT", "DELETE", "PATCH", "CONNECT")
		case "read":
			// store query params in global config mapped to route name
			CONFIG.QueryParams[r.Name] = r.QueryParams
			httpMethod := "GET"
			router.HandleFunc(
				r.URLScheme,
				AuthorizeReq(pool, WrapQuery(pool))).
				Name(r.Name).
				Methods(httpMethod)
		case "create", "update", "delete":
			httpMethod := ""
			switch r.Type {
			case "create":
				httpMethod = "POST"
			case "update":
				httpMethod = "PUT"
			default:
				httpMethod = "DELETE"
			}
			router.HandleFunc(r.URLScheme, AuthorizeReq(pool, WrapExec(pool))).
				Name(r.Name).
				Methods(httpMethod)
		case "transaction":
			router.HandleFunc(
				r.URLScheme,
				AuthorizeReq(pool, WrapTransaction(pool))).
				Name(r.Name).
				Methods("POST", "PUT", "DELETE")
		default:
		}
	}
	return err
}

func CreateServiceFunc(prefix string, wrapped func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
		wrapped(w, req)
	}
}

func TeeError(w http.ResponseWriter, err error) {
	log.Println(err)
	w.WriteHeader(http.StatusInternalServerError)
	if CONFIG.Debug {
		w.Write(FormatErr(err.Error()))
	}
}

func AuthorizeReq(pool *pgxpool.Pool, wrapped func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	CrudMap := make(map[string]string)
	CrudMap[http.MethodGet] = "select"
	CrudMap[http.MethodPost] = "insert"
	CrudMap[http.MethodPut] = "update"
	CrudMap[http.MethodDelete] = "delete"
	// NOTE http connect is more complicated
	// TODO consider removing
	CrudMap[http.MethodConnect] = "service"
	return func(w http.ResponseWriter, r *http.Request) {
		currentRoute := mux.CurrentRoute(r)
		routeName := currentRoute.GetName()
		isServiceReq, err := IsServiceRequest(currentRoute)
		if err != nil {
			TeeError(w, err)
			return
		}
		reqType := CrudMap[r.Method]
		if isServiceReq {
			reqType = "service"
		}
		authPath := fmt.Sprintf(
			"%s/auth/%s/%s.sql",
			CONFIG.SQLRoot,
			reqType,
			routeName)
		authPath = filepath.Clean(authPath)
		q, err := os.ReadFile(authPath)
		if err != nil {
			TeeError(w, err)
			return
		}
		params, err := ExtractParams(r)
		if err != nil {
			TeeError(w, err)
			return
		}
		log.Println("authorizing", r.Method, routeName, params)
		var isAuthorized bool
		tx, err := pool.Begin(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		err = SetLocalParams(&tx, r)
		if err != nil {
			TeeError(w, err)
			return
		}
		err = tx.QueryRow(
			context.Background(),
			string(q),
			params...).
			Scan(&isAuthorized)
		if err != nil {
			TeeError(w, err)
			return
		}
		err = tx.Commit(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		if isAuthorized {
			wrapped(w, r)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	}
}

func IsServiceRequest(r *mux.Route) (bool, error) {
	result := false
	methods, err := r.GetMethods()
	if err != nil {
		return result, err
	}
	for _, method := range methods {
		if method == "CONNECT" {
			return true, err
		}
	}
	return result, err
}

func WrapQuery(pool *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		routeName := mux.CurrentRoute(r).GetName()
		params, err := ExtractParams(r)
		if err != nil {
			TeeError(w, err)
			return
		}
		log.Println("processing", r.Method, routeName, params)
		tx, err := pool.Begin(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		err = SetLocalParams(&tx, r)
		if err != nil {
			TeeError(w, err)
			return
		}
		result, n, err := ExecQuery(&tx, r.Method, routeName, params)
		if err != nil {
			TeeError(w, err)
			return
		}
		err = tx.Commit(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		if len(result) == 0 {
			if n > 0 {
				// a json_agg result
				result = []byte("[]")
			} else {
				// a row_to_json result
				w.WriteHeader(http.StatusNotFound)
			}
		}
		w.Write(result)
	}
}

func SetLocalParams(tx *pgx.Tx, r *http.Request) error {
	appUserAuth, err := GetAppUserAuth(r)
	if err != nil {
		return err
	}
	q := "select set_config('app_user.auth',$1,true)"
	_, err = (*tx).Exec(context.Background(), q, appUserAuth)
	if err != nil {
		return err
	}
	appUserCookies, err := GetJSONFromCookies(r.Cookies())
	q = "select set_config('app_user.cookies',$1,true)"
	_, err = (*tx).Exec(context.Background(), q, appUserCookies)
	if err != nil {
		return err
	}
	// get system user for file path expansion
	var result string
	var byt []byte
	for k, v := range CONFIG.AppUserLocalParams {
		byt, err = os.ReadFile(v)
		if err != nil {
			return err
		}
		err := (*tx).QueryRow(context.Background(), string(byt)).Scan(&result)
		if err != nil {
			return err
		}
		log.Println(k, result)
		q = fmt.Sprintf("select set_config('app_user.%s',$1,true)", k)
		_, err = (*tx).Exec(context.Background(), q, result)
		if err != nil {
			return err
		}
	}
	return err
}

func ExecQuery(tx *pgx.Tx, method string, routeName string, params []interface{}) ([]byte, int64, error) {
	var result []byte
	var n int64
	pathTmpl := ""
	switch method {
	case http.MethodGet:
		pathTmpl = "%s/select/%s.sql"
	default:
		return result, n, errors.New("invalid http method for query execution")
	}
	path := fmt.Sprintf(pathTmpl, CONFIG.SQLRoot, routeName)
	path = filepath.Clean(path)
	q, err := os.ReadFile(path)
	if err != nil {
		return result, n, err
	}
	log.Println("executing", path, params)
	rows, err := (*tx).Query(context.Background(), string(q), params...)
	defer rows.Close()
	if err != nil {
		return result, n, err
	}
	for rows.Next() {
		result = rows.RawValues()[0]
	}
	n = rows.CommandTag().RowsAffected()
	return result, n, err
}

func GetAppUserAuth(r *http.Request) (string, error) {
	result := ""
	var err error
	var segments []string
	var byt []byte
	if CONFIG.AppUserAuth["ParseFrom"] == "Header" {
		result = r.Header.Get(CONFIG.AppUserAuth["Field"])
		if CONFIG.AppUserAuth["Type"] == "JWT" {
			split := strings.Split(result, " ")
			segments = strings.Split(split[len(split)-1], ".")
			if len(segments) != 3 {
				return result, fmt.Errorf(
					"invalid JWT format. expected 3 segments, found %i",
					len(segments))
			}
			byt, err = base64.RawURLEncoding.DecodeString(segments[1])
			if err != nil {
				return result, err
			}
			if CONFIG.AppUserAuth["Claim"] == "" {
				return string(byt), err
			}
			mapped := make(map[string]interface{})
			err = json.Unmarshal(byt, &mapped)
			if err != nil {
				return result, err
			}
			if configClaim, ok := CONFIG.AppUserAuth["Claim"]; ok {
				if claim, ok := mapped[configClaim]; ok {
					result = claim.(string)
				}
			}
		}
	}
	if CONFIG.AppUserAuth["ParseFrom"] == "Cookie" {
		var userCookie *http.Cookie
		if CONFIG.AppUserAuth["Name"] == "" {
			return GetJSONFromCookies(r.Cookies())
		}
		userCookie, err = r.Cookie(CONFIG.AppUserAuth["Name"])
		if err != nil {
			return result, err
		}
		result = userCookie.Value
		if CONFIG.AppUserAuth["Type"] == "JWT" {
			segments = strings.Split(result, ".")
			if len(segments) != 3 {
				return result, fmt.Errorf(
					"invalid JWT format. expected 3 segments, found %d",
					len(segments))
			}
			byt, err = base64.RawURLEncoding.DecodeString(segments[1])
			if err != nil {
				return result, err
			}
			if CONFIG.AppUserAuth["Claim"] == "" {
				return string(byt), err
			}
			mapped := make(map[string]interface{})
			err = json.Unmarshal(byt, &mapped)
			if err != nil {
				return result, err
			}
			if configClaim, ok := CONFIG.AppUserAuth["Claim"]; ok {
				if claim, ok := mapped[configClaim]; ok {
					result = claim.(string)
				}
			}
		}
	}
	return result, err
}

func GetJSONFromCookies(cookies []*http.Cookie) (string, error) {
	byt, err := json.Marshal(GetMapFromCookies(cookies))
	return string(byt), err
}

func GetMapFromCookies(cookies []*http.Cookie) map[string]string {
	result := make(map[string]string)
	for _, cookie := range cookies {
		result[cookie.Name] = cookie.Value
	}
	return result
}

func WrapExec(pool *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		routeName := mux.CurrentRoute(r).GetName()
		pathTmpl := ""
		switch r.Method {
		case http.MethodPost:
			pathTmpl = "%s/insert/%s.sql"
		case http.MethodPut:
			pathTmpl = "%s/update/%s.sql"
		case http.MethodDelete:
			pathTmpl = "%s/delete/%s.sql"
		default:
			log.Println("HTTP Method not recognized.")
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		path := fmt.Sprintf(pathTmpl, CONFIG.SQLRoot, routeName)
		path = filepath.Clean(path)
		q, err := os.ReadFile(path)
		if err != nil {
			TeeError(w, err)
			return
		}
		params, err := ExtractParams(r)
		if err != nil {
			TeeError(w, err)
			return
		}
		log.Println("processing", r.Method, routeName, params)
		log.Println("executing", path, "with arguments", params)
		tx, err := pool.Begin(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		err = SetLocalParams(&tx, r)
		if err != nil {
			TeeError(w, err)
			return
		}
		rows, err := tx.Query(context.Background(), string(q), params...)
		defer rows.Close()
		if err != nil {
			TeeError(w, err)
			return
		}
		var route Route
		for _, route = range CONFIG.Routes {
			if route.Name == routeName && route.Type == "read" {
				break
			}
		}
		// non-greedy capturing with (?U)
		re := regexp.MustCompile(`(?U){(.*)}`)
		groups := re.FindAllStringSubmatch(route.URLScheme, -1)
		var pathVars []string
		for _, val := range groups {
			pathVars = append(pathVars, val[1])
		}
		params = params[:0]
		var returnMap map[string]interface{}
		var rawResult []json.RawMessage
		// NOTE rows.RawValues are only valid until next call to Next
		var rawValue []byte
		var rawValues [][]byte
		for rows.Next() {
			rawValue = make([]byte, len(rows.RawValues()[0]))
			_ = copy(rawValue, rows.RawValues()[0])
			rawValues = append(rawValues, rawValue)
		}
		// NOTE RowsAffected is only known after all rows are read
		n := rows.CommandTag().RowsAffected()
		rows.Close()
		// TODO refactor: violates linux style guide nesting recommendation
		for _, rawValue := range rawValues {
			params = params[:0]
			err = json.Unmarshal(rawValue, &returnMap)
			if err != nil {
				TeeError(w, err)
				return
			}
			for _, p := range pathVars {
				params = append(params, returnMap[p])
			}
			for i, q := range CONFIG.QueryParams[routeName] {
				if i%2 != 0 {
					continue
				}
				str := pgtype.Text{String: "", Valid: false}
				for k, v := range returnMap {
					if q == k {
						b, err := json.Marshal(v)
						if err != nil {
							TeeError(w, err)
							return
						}
						str.String = string(b)
						str.Valid = true
						break
					}
				}
				params = append(params, str)
			}
			log.Println("returning", r.Method, routeName, params)
			result, _, err := ExecQuery(&tx, "GET", routeName, params)
			if err != nil {
				TeeError(w, err)
				return
			}
			c := make([]byte, len(result))
			copy(c, result)
			rawResult = append(rawResult, c)
		}
		err = tx.Commit(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		log.Println("rows affected:", n)
		if n == 0 {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
			return
		} else {
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		jsonResult, err := json.Marshal(rawResult)
		if err != nil {
			TeeError(w, err)
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		w.Write(jsonResult)
	}
}

func FormatErr(err string) []byte {
	return []byte(fmt.Sprintf(`{"error":%q}`, err))
}

func ExtractParams(r *http.Request) ([]interface{}, error) {
	var params []interface{}
	pathTemplate, err := mux.CurrentRoute(r).GetPathTemplate()
	if err != nil {
		return params, err
	}
	// non-greedy capturing with (?U)
	re := regexp.MustCompile(`(?U){(.*)}`)
	groups := re.FindAllStringSubmatch(pathTemplate, -1)
	var pathVars []string
	for _, val := range groups {
		pathVars = append(pathVars, val[1])
	}
	vars := mux.Vars(r)
	for _, v := range pathVars {
		params = append(params, vars[v])
	}
	if r.Method == http.MethodGet {
		if CONFIG.QueryStringAsJSON {
			queryMap, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil {
				return params, err
			}
			atomMap := make(map[string]string)
			for k, v := range queryMap {
				atomMap[k] = v[0]
			}
			j, err := json.Marshal(atomMap)
			if err != nil {
				return params, err
			}
			params = append(params, string(j))
		} else {
			routeName := mux.CurrentRoute(r).GetName()
			query := r.URL.Query()
			for i, queryParam := range CONFIG.QueryParams[routeName] {
				// TODO perform regex matching and validation
				if i%2 == 0 {
					str := pgtype.Text{String: query.Get(queryParam), Valid: true}
					if str.String == "" {
						str.Valid = false
					}
					params = append(params, str)
				}
			}
		}
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		arg, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		r.Body = ioutil.NopCloser(bytes.NewBuffer(arg))
		if err != nil {
			return params, err
		}
		params = append(params, string(arg))
	}
	return params, err
}

// NOTE for server side includes and rendering of static content based on user info
// NOTE the template handling is not intended to be a full on backend rendering engine
// NOTE the only template input is a map of user info
// NOTE depends on AppUserLocalParams["info"] for user details
// NOTE multiple template dirs and overlays are possible via template config
func HandleTemplateReq(pool *pgxpool.Pool, templateDir string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		params, err := ExtractParams(r)
		if err != nil {
			TeeError(w, err)
			return
		}
		var result []byte
		q, err := os.ReadFile(CONFIG.AppUserLocalParams["info"])
		if err != nil {
			TeeError(w, err)
			return
		}
		tx, err := pool.Begin(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		err = SetLocalParams(&tx, r)
		if err != nil {
			TeeError(w, err)
			return
		}
		rows, err := tx.Query(context.Background(), string(q), params...)
		if err != nil {
			TeeError(w, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			result = rows.RawValues()[0]
		}
		err = tx.Commit(context.Background())
		if err != nil {
			TeeError(w, err)
			return
		}
		log.Println(string(result))
		var appUser map[string]interface{}
		err = json.Unmarshal(result, &appUser)
		if err != nil {
			TeeError(w, err)
			return
		}
		funcMap := template.FuncMap{}
		funcMap["Title"] = strings.Title
		baseTmpl := templateDir + "/base.go.html"
		baseTmpl = filepath.Clean(baseTmpl)
		base, err := template.New("base.go.html").
			Funcs(funcMap).
			ParseFiles(baseTmpl)
		if err != nil {
			TeeError(w, err)
			return
		}
		reqTmpl := fmt.Sprintf(
			"%s/%s/index.go.html",
			templateDir,
			path.Clean(r.URL.EscapedPath()))
		reqTmpl = filepath.Clean(reqTmpl)
		overlay, err := template.Must(base.Clone()).ParseFiles(reqTmpl)
		if err != nil {
			TeeError(w, err)
			return
		}
		err = overlay.Execute(w, appUser)
		if err != nil {
			TeeError(w, err)
			return
		}
	}
}

// TODO refactor directory handling
// TODO parse each statement from transaction script
// TODO or get each statement from the ast
// TODO execute each statement use SendBatch
func WrapTransaction(pool *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		routeName := mux.CurrentRoute(r).GetName()
		manifestFilePath := fmt.Sprintf(
			"%s/transaction/%s/manifest.json",
			CONFIG.SQLRoot,
			routeName)
		manifestFh, err := os.Open(manifestFilePath)
		defer manifestFh.Close()
		if err != nil {
			log.Println(err)
			if CONFIG.Debug {
				w.Write(FormatErr(err.Error()))
			}
			return
		}
		scanner := bufio.NewScanner(manifestFh)
		// TODO use ExtractParams
		appUserAuth, err := GetAppUserAuth(r)
		if err != nil {
			log.Println(err)
			if CONFIG.Debug {
				w.Write(FormatErr(err.Error()))
			}
			return
		}
		arg, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Println(err)
			if CONFIG.Debug {
				w.Write(FormatErr(err.Error()))
			}
			return
		}
		tx, err := pool.Begin(context.Background())
		if err != nil {
			log.Println(err)
			if CONFIG.Debug {
				w.Write(FormatErr(err.Error()))
			}
			return
		}
		for scanner.Scan() {
			fileName := scanner.Text()
			path := fmt.Sprintf(
				"%s/transaction/%s/%s",
				CONFIG.SQLRoot,
				routeName,
				fileName)
			path = filepath.Clean(path)
			q, err := os.ReadFile(path)
			_, err = tx.Exec(
				context.Background(),
				string(q),
				appUserAuth,
				arg)
			if err != nil {
				_ = tx.Rollback(context.Background())
				log.Println(err)
				if CONFIG.Debug {
					w.Write(FormatErr(err.Error()))
				}
				return
			}
		}
		err = tx.Commit(context.Background())
		if err != nil {
			log.Println(err)
			if CONFIG.Debug {
				w.Write(FormatErr(err.Error()))
			}
		}
	}
}
