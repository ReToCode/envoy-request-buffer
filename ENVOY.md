# PoC: Running the PoC locally on envoy in docker-compose

Please note: this version has a static control-plane where service status is managed manually.
The [kubernetes](./kubernetes) version has automatic detection of scaled-to-zero services.

## Demo

<a href="https://asciinema.org/a/640583?autoplay=1" target="_blank"><img src="https://asciinema.org/a/640583.svg" /></a>

For more info about the demo see the scripts in [demo](./local-envoy/demo)

## Local setup

```bash
brew tap tinygo-org/tools
brew install tinygo
```

## Ports

* 7001: [control plane](./control-plane/main.go)
* 8001: Envoy admin endpoint
* 9000: Envoy traffic endpoint (hosts: `http.example.com`, `grpc.example.com`)
* 50001: [upstream backend (http)](./upstream/main.go)
* 50002: [upstream backend (grpc)](./upstream/main.go)

## Building + running

```bash
make build-wasm && docker-compose -f local-envoy/docker-compose.yaml up
```

## Setting hosts to scaled to zero

The control-plane returns a list of which services are "scaled-to-zero" (dummy):

```bash
# Get current list
curl localhost:7001/ 
[]
```

```bash
# Setting a service to "scaled to zero" or unset "scaled to zero"
curl -X POST "localhost:7001/set-scaled-to-zero?host=http.example.com"
curl -X POST "localhost:7001/set-scaled-to-zero?host=grpc.example.com"
```

## Testing directly

```bash
# HTTP
curl localhost:50001
Hello from HTTP Server
```

```bash
# GRPC
grpcurl -plaintext localhost:50002 grpc.health.v1.Health/Check
{
  "status": "SERVING"
}
```

## Testing via Envoy

```bash
# HTTP
curl localhost:9000 -H 'Host: http.example.com'
Hello from HTTP Server
```

```bash
# GRPC
grpcurl -plaintext -authority grpc.example.com localhost:9000 grpc.health.v1.Health/Check
{
  "status": "SERVING"
}
```

