package main

import (
	"github.com/retocode/envoy-request-buffer/wasm-request-buffer/filter"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const tickMilliseconds uint32 = 1000

type vmContext struct {
	types.DefaultVMContext
}

type pluginContext struct {
	types.DefaultPluginContext
	contextID uint32
}

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

func (ctx *pluginContext) NewTcpContext(contextID uint32) types.TcpContext {
	return &filter.NetworkContext{}
}
