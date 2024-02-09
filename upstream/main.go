package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

var (
	httpPort = flag.Int("http-port", 50001, "The plain http server port")
	grpcPort = flag.Int("grpc-port", 50002, "The GRPC server port")
)

func main() {
	go httpServer()
	go grpcServer()
	select {}
}

func httpServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from HTTP Server"))
	})
	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(*httpPort),
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}
	log.Printf("Starting HTTP server on port: %d", *httpPort)
	log.Fatal(srv.ListenAndServe())
}

func grpcServer() {
	g := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(g, health.NewServer())
	reflection.Register(g)

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(*grpcPort))
	if err != nil {
		log.Fatalf("failed to listen for grpc server: %v", err)
	}

	log.Printf("Starting GRPC server on port: %d", *grpcPort)

	if err = g.Serve(lis); err != nil {
		log.Fatalf("failed to serve grpc server: %v", err)
	}
}
