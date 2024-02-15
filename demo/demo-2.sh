#!/usr/bin/env bash

. demo-magic.sh -n

clear

wait

pe "# this request will now be held"

pe "curl localhost:9000 -H 'Host: http.example.com'"

pe "# the request was forwarded"
