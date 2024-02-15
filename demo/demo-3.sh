#!/usr/bin/env bash

. demo-magic.sh -n

clear

wait

pe "# this request will now be held"

pe "grpcurl -plaintext -authority grpc.example.com localhost:9000 grpc.health.v1.Health/Check"

pe "# the request was forwarded"
