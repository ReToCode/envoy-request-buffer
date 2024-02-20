package main

import (
	"encoding/json"

	"github.com/retocode/envoy-request-buffer/wasm-request-buffer/shared"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const tickMilliseconds uint32 = 1000 * 2 // every 2 seconds

type vmContext struct {
	types.DefaultVMContext
}

type servicePluginContext struct {
	contextID uint32
	config    *shared.PluginConfig
	types.DefaultPluginContext
}

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

func (*vmContext) OnVMStart(vmConfigurationSize int) types.OnVMStartStatus {
	// Set the initial value of the shared data to an empty slice
	b := shared.EncodeSharedData([]string{})
	if err := proxywasm.SetSharedData(shared.ScaledToZeroClustersKey, b, 0); err != nil {
		proxywasm.LogCriticalf("error setting shared data on OnVMStart: %v", err)
		return types.OnVMStartStatusFailed
	}
	return types.OnVMStartStatusOK
}

func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &servicePluginContext{
		contextID: contextID,
	}
}

// NewHttpContext is only necessary because istio does not (yet) support a Kind: WasmModule with type WasmService
func (ctx *servicePluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &types.DefaultHttpContext{}
}

func (ctx *servicePluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	data, err := proxywasm.GetPluginConfiguration()
	if err != nil {
		proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	config, err := shared.ParseConfig(data)
	if err != nil {
		proxywasm.LogCriticalf("failed to parse plugin config: %s, err: %v", data, err)
		return types.OnPluginStartStatusFailed
	}
	ctx.config = config

	// Start a ticker to get status from control-plane
	if err := proxywasm.SetTickPeriodMilliSeconds(tickMilliseconds); err != nil {
		proxywasm.LogCriticalf("failed to set tick period: %v", err)
		return types.OnPluginStartStatusFailed
	}

	proxywasm.LogInfo("Service plugin started with ticker")
	return types.OnPluginStartStatusOK
}

func (ctx *servicePluginContext) OnTick() {
	// Call our control plane to get the new list of scaled to zero clusters
	headers := [][2]string{
		{":method", "GET"},
		{":authority", ctx.config.ControlPlaneURL},
		{":path", "/"},
		{"accept", "*/*"},
	}

	// TODO: 7001 is hardcoded for now, make it configurable
	// outbound|7001||control-plane.default.svc.cluster.local
	clusterName := "outbound|7001||" + ctx.config.ControlPlaneURL
	proxywasm.LogInfof("calling out to %s with headers: %v", clusterName, headers)

	if _, err := proxywasm.DispatchHttpCall(clusterName, headers, nil, nil,
		5000, ctx.controlPlaneResponseCallback); err != nil {
		proxywasm.LogCriticalf("dispatch httpcall failed: %v", err)
	}
}

func (ctx *servicePluginContext) controlPlaneResponseCallback(numHeaders, bodySize, numTrailers int) {
	b, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
	if err != nil {
		proxywasm.LogCriticalf("failed to get control-plane response body: %v", err)
		return
	}

	proxywasm.LogInfof("Received from control-plane: %s", b)

	var currentScaledToZeroClusters []string
	err = json.Unmarshal(b, &currentScaledToZeroClusters)
	if err != nil {
		proxywasm.LogCriticalf("failed to parse control-plane response body: %v", err)
		return
	}

	// 1) update the shared state with all currently scaled to zero clusters
	proxywasm.LogInfof("Persisting %d paused clusters to the shared state", len(currentScaledToZeroClusters))
	clustersEncoded := shared.EncodeSharedData(currentScaledToZeroClusters)
	if err := proxywasm.SetSharedData(shared.ScaledToZeroClustersKey, clustersEncoded, 0); err != nil {
		proxywasm.LogCriticalf("error setting shared data: %v", err)
		return
	}
}
