# Testing on Kubernetes

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
kubectl apply -f k8s/yaml/gateway.yaml
kubectl apply -f k8s/yaml/grpc-upstream.yaml
kubectl apply -f k8s/yaml/http-upstream.yaml
kubectl apply -f k8s/yaml/control-plane.yaml
```

## Check if it is working

```bash
curl http://http.172.17.0.100.sslip.io
```
```text
Hello from HTTP Server
```

```bash
grpcurl -plaintext grpc.172.17.0.100.sslip.io:80 grpc.health.v1.Health/Check`
```
```text
{
  "status": "SERVING"
}
```

## Setting clusters to or not to scaled to zero

```bash
curl -X POST "control-plane.172.17.0.100.sslip.io/set-scaled-to-zero?host=http.172.17.0.100.sslip.io"
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

## Adding the WASM module

```bash
kubectl apply -f k8s/yaml/wasm-plugin-request-buffer.yaml
```
