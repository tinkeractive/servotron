// TODO MySQL support
// TODO ingest OpenAPI specs for routes
// TODO connection pools for multiple databases (reader, writer)
// TODO consider prepared stmt handling

package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

func main() {
	configFilePath := flag.String("config", "", "config file path")
	flag.Parse()
	configBytes, err := os.ReadFile(*configFilePath)
	if err != nil {
		log.Fatal(err)
	}
	// TODO remove global CONFIG
	// TODO replace with local cfg and propagate servotron config
	cfg := Config{}
	err = cfg.Parse(configBytes)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("config:", cfg.String())
	servo, err := NewServotron(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(
		"connected to database",
		servo.pool.Config().ConnConfig.Database,
		"at host",
		servo.pool.Config().ConnConfig.Host,
		"on port",
		servo.pool.Config().ConnConfig.Port,
		"as user",
		servo.pool.Config().ConnConfig.User)
	log.Println("db pool size:", servo.pool.Config().MinConns)
	// management server listening for admin requests on management port
	mgmtRouter := mux.NewRouter()
	mgmtRouter.HandleFunc("/routes", servo.LoadRoutesHandler).Methods("POST")
	mgmtServer := &http.Server{
		Handler: mgmtRouter,
		Addr:    ":" + servo.config.ManagementPort,
	}
	log.Println("listening on management port", servo.config.ManagementPort)
	go mgmtServer.ListenAndServe()
	// server
	log.Println("listening on port", servo.config.ListenPort)
	log.Fatal(servo.server.ListenAndServe())
}
