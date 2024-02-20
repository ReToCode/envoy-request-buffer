#!/usr/bin/env bash

. demo-magic.sh -n

clear

wait

pe "# envoy proxy, control-plane and upstream services are running in Kubernetes"
pe "# Let's see the status of the deployments:"

pe "kubectl get deploy http-upstream grpc-upstream -n default"

pe "# as you can see, both upstream targets are ready+healthy. Let's call them"

pe "curl http://http.172.17.0.100.sslip.io"
pei ""

pe "grpcurl -plaintext -authority grpc.172.17.0.100.sslip.io grpc.172.17.0.100.sslip.io:80 grpc.health.v1.Health/Check"

pe "# Now let's scale them down to zero"

pe "kubectl scale deploy/grpc-upstream -n default --replicas=0"

pe "kubectl scale deploy/http-upstream -n default --replicas=0"

pe "# in the other terminal, we'll watch the deployments"

wait

pe "# now let's call the scaled-to-zero services again"
pe "# make sure to watch the other terminal"

pe "curl http://http.172.17.0.100.sslip.io"
pei ""

pe "grpcurl -plaintext -authority grpc.172.17.0.100.sslip.io grpc.172.17.0.100.sslip.io:80 grpc.health.v1.Health/Check"

wait

pe "# That's it, scale from/to zero with an envoy WASM extension 🎉 in Kubernetes"

