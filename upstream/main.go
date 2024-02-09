package main

import (
	"flag"
	"log"
	"net"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

var (
	httpPort = flag.Int("http-port", 50001, "The plain http server port")
	grpcPort = flag.Int("grpc-port", 50002, "The GRPC server port")
)

func main() {
	go grpcServer()
	select {}
}

func grpcServer() {
	g := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(g, health.NewServer())

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(*grpcPort))
	if err != nil {
		log.Fatalf("failed to listen for grpc server: %v", err)
	}

	log.Printf("Starting GRPC server on port: %d", *grpcPort)

	if err = g.Serve(lis); err != nil {
		log.Fatalf("failed to serve grpc server: %v", err)
	}
}
