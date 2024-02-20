#!/usr/bin/env bash

. demo-magic.sh -n

clear

wait

pe "watch kubectl get deploy http-upstream grpc-upstream -n default"

