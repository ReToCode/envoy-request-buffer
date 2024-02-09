package filter

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

type networkContext struct {
	types.DefaultTcpContext
	pluginCtx *pluginContext
}

func (ctx *networkContext) OnNewConnection() types.Action {
	proxywasm.LogInfo("new connection!")

	configuration, _ := proxywasm.GetVMConfiguration()
	proxywasm.LogInfof("vmconfig: %v", configuration)

	return types.ActionContinue
}

func (ctx *networkContext) OnDownstreamData(dataSize int, endOfStream bool) types.Action {
	if dataSize == 0 {
		return types.ActionContinue
	}

	proxywasm.LogInfof(">>>>>> downstream data received >>>>>>\n")
	return types.ActionContinue
}

func (ctx *networkContext) OnDownstreamClose(types.PeerType) {
	proxywasm.LogInfo("downstream connection close!")
	return
}

func (ctx *networkContext) OnUpstreamData(dataSize int, endOfStream bool) types.Action {
	if dataSize == 0 {
		return types.ActionContinue
	}

	// Get the remote ip address of the upstream cluster.
	address, err := proxywasm.GetProperty([]string{"upstream", "address"})
	if err != nil {
		proxywasm.LogWarnf("failed to get upstream remote address: %v", err)
	}

	proxywasm.LogInfof("<<<<<< upstream data received from: %s <<<<<<\n", string(address))
	return types.ActionContinue
}

func (ctx *networkContext) OnStreamDone() {
	proxywasm.LogInfo("connection complete!")
}
