target-dir:
	mkdir -p target

build: target-dir build-service build-filter

build-service:
	tinygo build -o ./target/request_buffer_service.wasm -scheduler=none -target=wasi wasm-request-buffer/service/main.go

build-filter:
	tinygo build -o ./target/request_buffer_filter.wasm -scheduler=none -target=wasi wasm-request-buffer/filter/main.go
