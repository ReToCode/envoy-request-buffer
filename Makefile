target-dir:
	mkdir -p local-envoy/target

build-wasm: target-dir build-wasm-service build-wasm-filter

build-wasm-service:
	tinygo build -o ./local-envoy/target/request_buffer_service.wasm -scheduler=none -target=wasi wasm-request-buffer/service/main.go

build-wasm-filter:
	tinygo build -o ./local-envoy/target/request_buffer_filter.wasm -scheduler=none -target=wasi wasm-request-buffer/filter/main.go

docker: docker-build docker-push docker-wasm

docker-wasm: docker-build-wasm docker-push-wasm

docker-build:
	docker build -t quay.io/rlehmann/upstream:latest . -f upstream/Containerfile
	docker build -t quay.io/rlehmann/control-plane:latest . -f kubernetes/control-plane/Containerfile

docker-build-wasm:
	docker build -t quay.io/rlehmann/envoy-request-buffer-wasm-service:latest . -f wasm-request-buffer/Containerfile_service
	docker build -t quay.io/rlehmann/envoy-request-buffer-wasm-filter:latest . -f wasm-request-buffer/Containerfile_filter

docker-push:
	docker push quay.io/rlehmann/upstream:latest
	docker push quay.io/rlehmann/control-plane:latest

docker-push-wasm:
	docker push quay.io/rlehmann/envoy-request-buffer-wasm-service:latest
	docker push quay.io/rlehmann/envoy-request-buffer-wasm-filter:latest

