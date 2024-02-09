#

## Local setup

```bash
brew tap tinygo-org/tools
brew install tinygo
```

## Testing

Test GRPC server directly

```bash
grpcurl -plaintext localhost:50002 grpc.health.v1.Health/Check
{
  "status": "SERVING"
}
```

Test GRPC server via envoy

```bash
grpcurl -plaintext localhost:8080 grpc.health.v1.Health/Check
{
  "status": "SERVING"
}
```