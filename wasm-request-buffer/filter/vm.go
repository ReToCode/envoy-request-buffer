package filter

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

type VmContext struct {
	types.DefaultVMContext
}

func (*VmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{
		contextID:            contextID,
		scaledToZeroClusters: make(map[string][]uint32),
	}
}
