package main

import (
	"github.com/retocode/envoy-request-buffer/wasm-request-buffer/filter"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
)

func main() {
	proxywasm.SetVMContext(&filter.VmContext{})
}
