package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

var scaledToZeroClusters = make(map[string]struct{})

const httpPort = 7001

func main() {
	http.HandleFunc("/", getScaledToZeroClusters)
	http.HandleFunc("/set-scaled-to-zero", setScaledToZero)
	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(httpPort),
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}
	log.Printf("Starting HTTP server on port: %d\n", httpPort)
	log.Fatal(srv.ListenAndServe())
}

func setScaledToZero(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		log.Printf("failed to POST parse form: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hostname := r.Form.Get("host")

	if hostname == "" {
		log.Println("host not specified")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if _, has := scaledToZeroClusters[hostname]; has {
		log.Printf("Removing %s from scaledToZeroClusters\n", hostname)
		delete(scaledToZeroClusters, hostname)
	} else {
		log.Printf("Adding %s to scaledToZeroClusters\n", hostname)
		scaledToZeroClusters[hostname] = struct{}{}
	}

	w.WriteHeader(http.StatusOK)
}

func getScaledToZeroClusters(w http.ResponseWriter, r *http.Request) {
	keys := make([]string, 0, len(scaledToZeroClusters))
	for k := range scaledToZeroClusters {
		keys = append(keys, k)
	}
	jsonStr, err := json.Marshal(keys)
	if err != nil {
		log.Println("failed to marshal scaledToZeroClusters, err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	_, err = w.Write(jsonStr)
	if err != nil {
		log.Printf("failed to write to output stream, err: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
