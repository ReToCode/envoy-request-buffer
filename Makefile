target-dir:
	mkdir -p target

build: target-dir build-service build-filter

build-service:
	tinygo build -o ./target/request_buffer_service.wasm -scheduler=none -target=wasi wasm-request-buffer/service/main.go

build-filter:
	tinygo build -o ./target/request_buffer_filter.wasm -scheduler=none -target=wasi wasm-request-buffer/filter/main.go

docker: docker-build docker-push

docker-build:
	docker build -t quay.io/rlehmann/upstream:latest . -f upstream/Containerfile
	docker build -t quay.io/rlehmann/control-plane:latest . -f control-plane/Containerfile
	docker build -t quay.io/rlehmann/envoy-request-buffer-wasm-service:latest . -f wasm-request-buffer/Containerfile_service
	docker build -t quay.io/rlehmann/envoy-request-buffer-wasm-filter:latest . -f wasm-request-buffer/Containerfile_filter

docker-push:
	docker push quay.io/rlehmann/upstream:latest
	docker push quay.io/rlehmann/control-plane:latest
	docker push quay.io/rlehmann/envoy-request-buffer-wasm-service:latest
	docker push quay.io/rlehmann/envoy-request-buffer-wasm-filter:latest
