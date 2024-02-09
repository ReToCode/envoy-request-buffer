target-dir:
	mkdir -p target

build: build-wasm

build-wasm: target-dir
	tinygo build -o ./target/request_buffer.wasm -scheduler=none -target=wasi wasm-request-buffer/main.go

