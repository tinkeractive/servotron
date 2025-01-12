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

	"github.com/tinkeractive/servotron"
	"github.com/gorilla/mux"
)

func main() {
	configFilePath := flag.String("config", "", "config file path")
	flag.Parse()
	configBytes, err := os.ReadFile(*configFilePath)
	if err != nil {
		log.Fatal(err)
	}
	cfg := servotron.Config{}
	err = cfg.Parse(configBytes)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("config:", cfg.String())
	servo, err := servotron.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("connected to database", cfg.DBConnString)
	// management server listening for admin requests on management port
	mgmtRouter := mux.NewRouter()
	mgmtRouter.HandleFunc("/routes", servo.LoadRoutesHandler).Methods("POST")
	mgmtServer := &http.Server{
		Handler: mgmtRouter,
		Addr:    ":" + cfg.ManagementPort,
	}
	log.Println("listening on management port", cfg.ManagementPort)
	go mgmtServer.ListenAndServe()
	// server
	log.Println("listening on port", cfg.ListenPort)
	log.Fatal(servo.ListenAndServe())
}
