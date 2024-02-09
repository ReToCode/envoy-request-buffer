package filter

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

type NetworkContext struct {
	types.DefaultTcpContext
}

func (ctx *NetworkContext) OnNewConnection() types.Action {
	proxywasm.LogInfo("new connection!")

	return types.ActionContinue
}

func (ctx *NetworkContext) OnDownstreamData(dataSize int, endOfStream bool) types.Action {
	if dataSize == 0 {
		return types.ActionContinue
	}

	proxywasm.LogInfof(">>>>>> downstream data received >>>>>>\n")
	return types.ActionContinue
}

func (ctx *NetworkContext) OnDownstreamClose(types.PeerType) {
	proxywasm.LogInfo("downstream connection close!")
	return
}

func (ctx *NetworkContext) OnUpstreamData(dataSize int, endOfStream bool) types.Action {
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

func (ctx *NetworkContext) OnStreamDone() {
	proxywasm.LogInfo("connection complete!")
}
