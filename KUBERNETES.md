# Testing on Kubernetes

## Demo

<a href="https://asciinema.org/a/641795" target="_blank"><img src="https://asciinema.org/a/641795.svg" /></a>

For more info about the demo see the scripts in [demo](./kubernetes/demo)


## Building

```bash
make docker
```

## Setup Istio with K8s Gateway API

```bash
# Install K8s Gateway API
kubectl get crd gateways.gateway.networking.k8s.io &> /dev/null || \
  { kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v1.0.0" | kubectl apply -f -; }

# Install istio as ingress solution (no mesh)
istioctl install --set profile=minimal -y
```

## Create Gateway API resources

```bash
kubectl apply -f kubernetes/yaml/gateway.yaml
kubectl apply -f kubernetes/yaml/grpc-upstream.yaml
kubectl apply -f kubernetes/yaml/http-upstream.yaml
kubectl apply -f kubernetes/yaml/control-plane.yaml
```

## Check if it is working

```bash
curl http://http.172.17.0.100.sslip.io
```
```text
Hello from HTTP Server
```

```bash
grpcurl -plaintext -authority grpc.172.17.0.100.sslip.io grpc.172.17.0.100.sslip.io:80 grpc.health.v1.Health/Check
```
```text
{
  "status": "SERVING"
}
```

## Adding the WASM module

```bash
kubectl apply -f kubernetes/yaml/wasm-plugin-request-buffer.yaml
```

## Scale the deployments to zero

```bash
kubectl scale deploy/grpc-upstream -n default --replicas=0
kubectl scale deploy/http-upstream -n default --replicas=0
```

## Send requests again and watch scale-from-zero

```bash
watch kubectl get deploy http-upstream grpc-upstream -n default
```

```bash
curl http://http.172.17.0.100.sslip.io
grpcurl -plaintext -authority grpc.172.17.0.100.sslip.io grpc.172.17.0.100.sslip.io:80 grpc.health.v1.Health/Check
```


## Debugging

```bash
# get envoy config
istioctl proxy-config all -n default deploy/external-gateway-istio -o json | copyfile
```
```bash
# set debug log in WASM
istioctl proxy-config log deploy/external-gateway-istio -n default --level "wasm:debug"
```

