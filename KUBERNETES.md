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

## Inspect the envoy configuration

```bash
istioctl proxy-config all -n default external-gateway-istio-7cd98768fc-g95gh -o json | copyfile
```

## Adding the WASM module

```bash
kubectl apply -f k8s/yaml/wasm-plugin-service.yaml
```
