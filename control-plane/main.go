package main

import (
	"log"
	"net/http"
	"strconv"
	"time"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from HTTP Server"))
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(*httpPort),
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}
	log.Printf("Starting HTTP server on port: %d", *httpPort)
	log.Fatal(srv.ListenAndServe())
}
