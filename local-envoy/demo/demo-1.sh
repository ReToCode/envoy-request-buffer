#!/usr/bin/env bash

. demo-magic.sh -n

clear

wait

pe "# envoy proxy, control-plane and upstream services are running"
pe "# let's send a http and a gRPC request directly to the services (without envoy)"

pe "curl localhost:50001"

pei ""

pe "grpcurl -plaintext localhost:50002 grpc.health.v1.Health/Check"

pe "# now let's call them through envoy (services are not scaled to zero)"

pe "curl localhost:9000 -H 'Host: http.example.com'"

pei ""

pe "grpcurl -plaintext -authority grpc.example.com localhost:9000 grpc.health.v1.Health/Check"

pe "# now let's indicate on the control-plane that these services are scaled to zero"

pe "curl -X POST \"localhost:7001/set-scaled-to-zero?host=http.example.com\""
pe "curl -X POST \"localhost:7001/set-scaled-to-zero?host=grpc.example.com\""

pe "# the control-plane now returns a list of scaled-to-zero services to envoy"

pe "curl localhost:7001/"

pei ""

pe "# envoy will now hold the request until the control-plane reports them to be scaled up again"

pe "# we'll execute the requests in the other terminal tabs"
wait

pe "# now let's indicate on the control-plane that these services are back up"

pe "# make sure to watch the two other terminal tabs"

pe "curl -X POST \"localhost:7001/set-scaled-to-zero?host=http.example.com\""
pe "curl -X POST \"localhost:7001/set-scaled-to-zero?host=grpc.example.com\""

pe "# this will now take about 5 secs to release the requests (can be configured)"

wait

pe "# That's it, scale from/to zero with an envoy WASM extension 🎉"

