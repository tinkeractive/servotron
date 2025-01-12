package servotron

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type servotron struct {
	config Config
	pool   *pgxpool.Pool
	router *mux.Router
	server *http.Server
}

func New(cfg Config) (servotron, error) {
	servo := servotron{config: cfg}
	pgxpoolConfig, err := pgxpool.ParseConfig(servo.config.DBConnString)
	if err != nil {
		return servo, err
	}
	pgxpoolConfig.MinConns = int32(servo.config.DBPoolSize)
	pgxpoolConfig.MaxConns = int32(servo.config.DBPoolSize)
	// TODO implement AfterConnect to check connections
	pool, err := pgxpool.NewWithConfig(context.Background(), pgxpoolConfig)
	if err != nil {
		return servo, err
	}
	servo.pool = pool
	servo.server = &http.Server{Addr: ":" + cfg.ListenPort}
	return servo, nil
}

func (s *servotron) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func (s *servotron) LoadRouter(routes []Route) error {
	router, err := s.CreateRouter(routes)
	if err != nil {
		log.Println(s.FormatErr(err.Error()))
		return err
	}
	s.router = router
	s.server.Handler = router
	return err
}

func (s *servotron) FormatErr(err string) []byte {
	return []byte(fmt.Sprintf(`{"error":%q}`, err))
}

func (s *servotron) LoadRoutesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("loading routes")
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(s.FormatErr(err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(s.FormatErr(err.Error()))
		return
	}
	routes, err := s.GetRoutesFromBytes(bytes)
	err = s.LoadRouter(routes)
	if err != nil {
		log.Println(s.FormatErr(err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(s.FormatErr(err.Error()))
		return
	}
	for _, route := range s.config.Routes {
		log.Println(route)
	}
	j, err := json.Marshal(s.config.Routes)
	if err != nil {
		log.Println(s.FormatErr(err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(s.FormatErr(err.Error()))
		return
	}
	w.Write(j)
}

func (s *servotron) GetRoutesFromBytes(bytes []byte) ([]Route, error) {
	var result []Route
	var err error
	if 0 < len(bytes) {
		err = json.Unmarshal(bytes, &result)
	}
	return result, err
}

func (s *servotron) CreateRouter(routes []Route) (*mux.Router, error) {
	router := mux.NewRouter()
	s.config.Routes = routes
	err := s.LoadRoutes(router, s.config.Routes)
	if err != nil {
		return router, err
	}
	for endpoint, dir := range s.config.FileServers {
		router.PathPrefix(endpoint).Handler(http.FileServer(http.Dir(dir)))
	}
	for endpoint, dir := range s.config.TemplateServers {
		router.PathPrefix(endpoint).Name(endpoint).HandlerFunc(s.HandleTemplateReq(dir))
	}
	return router, nil
}

func (s *servotron) LoadRoutes(router *mux.Router, routes []Route) error {
	err := error(nil)
	s.config.QueryParams = make(map[string][]string)
	for _, r := range routes {
		switch r.Type {
		case "service":
			serviceURL, err := url.Parse(r.ServiceURL)
			if err != nil {
				return err
			}
			// TODO comment
			serviceProxy := httputil.NewSingleHostReverseProxy(serviceURL)
			serviceAuthFunc := s.AuthorizeReq(serviceProxy.ServeHTTP)
			serviceFunc := s.CreateServiceFunc(r.URLScheme, serviceAuthFunc)
			router.PathPrefix(r.URLScheme).
				HandlerFunc(serviceFunc).
				Name(r.Name).
				Methods("GET", "POST", "PUT", "DELETE", "PATCH", "CONNECT")
		case "read":
			// store query params in global config mapped to route name
			s.config.QueryParams[r.Name] = r.QueryParams
			httpMethod := "GET"
			log.Println(r.Name)
			router.HandleFunc(r.URLScheme, s.AuthorizeReq(s.QueryHandler)).
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
			router.HandleFunc(r.URLScheme, s.AuthorizeReq(s.ExecHandler)).
				Name(r.Name).
				Methods(httpMethod)
		case "transaction":
			router.HandleFunc(
				r.URLScheme,
				s.AuthorizeReq(s.TransactionHandler)).
				Name(r.Name).
				Methods("POST", "PUT", "DELETE")
		default:
		}
	}
	return err
}

func (s *servotron) TeeError(w http.ResponseWriter, err error) {
	log.Println(err)
	w.WriteHeader(http.StatusInternalServerError)
	if s.config.Debug {
		w.Write(s.FormatErr(err.Error()))
	}
}

func (s *servotron) AuthorizeReq(wrapped func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
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
		isServiceReq, err := s.IsServiceRequest(currentRoute)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		reqType := CrudMap[r.Method]
		if isServiceReq {
			reqType = "service"
		}
		authPath := fmt.Sprintf(
			"%s/auth/%s/%s.sql",
			s.config.SQLRoot,
			reqType,
			routeName)
		authPath = filepath.Clean(authPath)
		q, err := os.ReadFile(authPath)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		params, err := s.ExtractParams(r)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		log.Println("authorizing", r.Method, routeName, params)
		var isAuthorized bool
		tx, err := s.pool.Begin(context.Background())
		if err != nil {
			s.TeeError(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		err = s.SetLocalParams(&tx, r)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		err = tx.QueryRow(
			context.Background(),
			string(q),
			params...).
			Scan(&isAuthorized)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		err = tx.Commit(context.Background())
		if err != nil {
			s.TeeError(w, err)
			return
		}
		if isAuthorized {
			wrapped(w, r)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	}
}

func (s *servotron) IsServiceRequest(r *mux.Route) (bool, error) {
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

func (s *servotron) CreateServiceFunc(prefix string, wrapped func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
		wrapped(w, req)
	}
}

func (s *servotron) QueryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	routeName := mux.CurrentRoute(r).GetName()
	params, err := s.ExtractParams(r)
	if err != nil {
		s.TeeError(w, err)
		return
	}
	log.Println("processing", r.Method, routeName, params)
	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		s.TeeError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	err = s.SetLocalParams(&tx, r)
	if err != nil {
		s.TeeError(w, err)
		return
	}
	result, n, err := s.ExecQuery(&tx, r.Method, routeName, params)
	if err != nil {
		s.TeeError(w, err)
		return
	}
	err = tx.Commit(context.Background())
	if err != nil {
		s.TeeError(w, err)
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

func (s *servotron) ExecQuery(tx *pgx.Tx, method string, routeName string, params []interface{}) ([]byte, int64, error) {
	var result []byte
	var n int64
	pathTmpl := ""
	switch method {
	case http.MethodGet:
		pathTmpl = "%s/select/%s.sql"
	default:
		return result, n, errors.New("invalid http method for query execution")
	}
	path := fmt.Sprintf(pathTmpl, s.config.SQLRoot, routeName)
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

func (s *servotron) ExecHandler(w http.ResponseWriter, r *http.Request) {
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
	path := fmt.Sprintf(pathTmpl, s.config.SQLRoot, routeName)
	path = filepath.Clean(path)
	q, err := os.ReadFile(path)
	if err != nil {
		s.TeeError(w, err)
		return
	}
	params, err := s.ExtractParams(r)
	if err != nil {
		s.TeeError(w, err)
		return
	}
	log.Println("processing", r.Method, routeName, params)
	log.Println("executing", path, "with arguments", params)
	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		s.TeeError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	err = s.SetLocalParams(&tx, r)
	if err != nil {
		s.TeeError(w, err)
		return
	}
	rows, err := tx.Query(context.Background(), string(q), params...)
	defer rows.Close()
	if err != nil {
		s.TeeError(w, err)
		return
	}
	var route Route
	for _, route = range s.config.Routes {
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
			s.TeeError(w, err)
			return
		}
		for _, p := range pathVars {
			params = append(params, returnMap[p])
		}
		for i, q := range s.config.QueryParams[routeName] {
			if i%2 != 0 {
				continue
			}
			str := pgtype.Text{String: "", Valid: false}
			for k, v := range returnMap {
				if q == k {
					b, err := json.Marshal(v)
					if err != nil {
						s.TeeError(w, err)
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
		result, _, err := s.ExecQuery(&tx, "GET", routeName, params)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		c := make([]byte, len(result))
		copy(c, result)
		rawResult = append(rawResult, c)
	}
	err = tx.Commit(context.Background())
	if err != nil {
		s.TeeError(w, err)
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
		s.TeeError(w, err)
		return
	}
	if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusCreated)
	}
	w.Write(jsonResult)
}

// TODO refactor directory handling
// TODO parse each statement from transaction script
// TODO or get each statement from the ast
// TODO execute each statement use SendBatch
func (s *servotron) TransactionHandler(w http.ResponseWriter, r *http.Request) {
	routeName := mux.CurrentRoute(r).GetName()
	manifestFilePath := fmt.Sprintf(
		"%s/transaction/%s/manifest.json",
		s.config.SQLRoot,
		routeName)
	manifestFh, err := os.Open(manifestFilePath)
	defer manifestFh.Close()
	if err != nil {
		log.Println(err)
		if s.config.Debug {
			w.Write(s.FormatErr(err.Error()))
		}
		return
	}
	scanner := bufio.NewScanner(manifestFh)
	// TODO use ExtractParams
	appUserAuth, err := s.GetAppUserAuth(r)
	if err != nil {
		log.Println(err)
		if s.config.Debug {
			w.Write(s.FormatErr(err.Error()))
		}
		return
	}
	arg, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		if s.config.Debug {
			w.Write(s.FormatErr(err.Error()))
		}
		return
	}
	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		log.Println(err)
		if s.config.Debug {
			w.Write(s.FormatErr(err.Error()))
		}
		return
	}
	for scanner.Scan() {
		fileName := scanner.Text()
		path := fmt.Sprintf(
			"%s/transaction/%s/%s",
			s.config.SQLRoot,
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
			if s.config.Debug {
				w.Write(s.FormatErr(err.Error()))
			}
			return
		}
	}
	err = tx.Commit(context.Background())
	if err != nil {
		log.Println(err)
		if s.config.Debug {
			w.Write(s.FormatErr(err.Error()))
		}
	}
}

func (s *servotron) ExtractParams(r *http.Request) ([]interface{}, error) {
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
		if s.config.QueryStringAsJSON {
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
			for i, queryParam := range s.config.QueryParams[routeName] {
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
		contentType := r.Header.Get("Content-Type")
		if strings.ToLower(contentType) == "application/json" {
			arg, err := ioutil.ReadAll(r.Body)
			r.Body.Close()
			r.Body = ioutil.NopCloser(bytes.NewBuffer(arg))
			if err != nil {
				return params, err
			}
			params = append(params, string(arg))
		}
	}
	return params, err
}

func (s *servotron) GetAppUserAuth(r *http.Request) (string, error) {
	result := ""
	var err error
	var segments []string
	var byt []byte
	if s.config.AppUserAuth["ParseFrom"] == "Header" {
		result = r.Header.Get(s.config.AppUserAuth["Field"])
		if s.config.AppUserAuth["Type"] == "JWT" {
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
			if s.config.AppUserAuth["Claim"] == "" {
				return string(byt), err
			}
			mapped := make(map[string]interface{})
			err = json.Unmarshal(byt, &mapped)
			if err != nil {
				return result, err
			}
			if configClaim, ok := s.config.AppUserAuth["Claim"]; ok {
				if claim, ok := mapped[configClaim]; ok {
					result = claim.(string)
				}
			}
		}
	}
	if s.config.AppUserAuth["ParseFrom"] == "Cookie" {
		var userCookie *http.Cookie
		if s.config.AppUserAuth["Name"] == "" {
			return s.GetJSONFromCookies(r.Cookies())
		}
		userCookie, err = r.Cookie(s.config.AppUserAuth["Name"])
		if err != nil {
			return result, err
		}
		result = userCookie.Value
		if s.config.AppUserAuth["Type"] == "JWT" {
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
			if s.config.AppUserAuth["Claim"] == "" {
				return string(byt), err
			}
			mapped := make(map[string]interface{})
			err = json.Unmarshal(byt, &mapped)
			if err != nil {
				return result, err
			}
			if configClaim, ok := s.config.AppUserAuth["Claim"]; ok {
				if claim, ok := mapped[configClaim]; ok {
					result = claim.(string)
				}
			}
		}
	}
	return result, err
}

func (s *servotron) GetMapFromCookies(cookies []*http.Cookie) map[string]string {
	result := make(map[string]string)
	for _, cookie := range cookies {
		result[cookie.Name] = cookie.Value
	}
	return result
}

func (s *servotron) GetJSONFromCookies(cookies []*http.Cookie) (string, error) {
	byt, err := json.Marshal(s.GetMapFromCookies(cookies))
	return string(byt), err
}

func (s *servotron) SetLocalParams(tx *pgx.Tx, r *http.Request) error {
	appUserAuth, err := s.GetAppUserAuth(r)
	if err != nil {
		return err
	}
	q := "select set_config('app_user.auth',$1,true)"
	_, err = (*tx).Exec(context.Background(), q, appUserAuth)
	if err != nil {
		return err
	}
	appUserCookies, err := s.GetJSONFromCookies(r.Cookies())
	q = "select set_config('app_user.cookies',$1,true)"
	_, err = (*tx).Exec(context.Background(), q, appUserCookies)
	if err != nil {
		return err
	}
	// get system user for file path expansion
	var result string
	var byt []byte
	for k, v := range s.config.AppUserLocalParams {
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

// NOTE for server side includes and rendering of static content based on user info
// NOTE the template handling is not intended to be a full on backend rendering engine
// NOTE the only template input is a map of user info
// NOTE depends on AppUserLocalParams["info"] for user details
// NOTE multiple template dirs and overlays are possible via template config
func (s *servotron) HandleTemplateReq(templateDir string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		params, err := s.ExtractParams(r)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		var result []byte
		q, err := os.ReadFile(s.config.AppUserLocalParams["info"])
		if err != nil {
			s.TeeError(w, err)
			return
		}
		tx, err := s.pool.Begin(context.Background())
		if err != nil {
			s.TeeError(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		err = s.SetLocalParams(&tx, r)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		rows, err := tx.Query(context.Background(), string(q), params...)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			result = rows.RawValues()[0]
		}
		err = tx.Commit(context.Background())
		if err != nil {
			s.TeeError(w, err)
			return
		}
		log.Println(string(result))
		var appUser map[string]interface{}
		err = json.Unmarshal(result, &appUser)
		if err != nil {
			s.TeeError(w, err)
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
			s.TeeError(w, err)
			return
		}
		reqTmpl := fmt.Sprintf(
			"%s/%s/index.go.html",
			templateDir,
			path.Clean(r.URL.EscapedPath()))
		reqTmpl = filepath.Clean(reqTmpl)
		overlay, err := template.Must(base.Clone()).ParseFiles(reqTmpl)
		if err != nil {
			s.TeeError(w, err)
			return
		}
		err = overlay.Execute(w, appUser)
		if err != nil {
			s.TeeError(w, err)
			return
		}
	}
}
