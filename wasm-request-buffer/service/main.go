package main

import (
	"encoding/json"
	"slices"

	"github.com/retocode/envoy-request-buffer/wasm-request-buffer/shared"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const tickMilliseconds uint32 = 1000 * 10 // every 5 seconds

type vmContext struct {
	types.DefaultVMContext
}

type servicePluginContext struct {
	contextID uint32
	types.DefaultPluginContext
	pausedClusters map[string][]uint32 // [hostname][httpContextIDs]
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
		contextID:      contextID,
		pausedClusters: make(map[string][]uint32),
	}
}

func (ctx *servicePluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	_, err := proxywasm.RegisterSharedQueue(shared.HTTPContextQueueName)
	if err != nil {
		proxywasm.LogCriticalf("error registering shared queue: %v", err)
		return types.OnPluginStartStatusFailed
	}

	// Start a ticker to get status from control-plane
	if err := proxywasm.SetTickPeriodMilliSeconds(tickMilliseconds); err != nil {
		proxywasm.LogCriticalf("failed to set tick period: %v", err)
		return types.OnPluginStartStatusFailed
	}

	proxywasm.LogInfo("Plugin successfully started")
	return types.OnPluginStartStatusOK
}

func (ctx *servicePluginContext) OnQueueReady(queueID uint32) {
	data, err := proxywasm.DequeueSharedQueue(queueID)
	switch err {
	case types.ErrorStatusEmpty:
		return
	case nil:
		ctx.onMessage(data)
	default:
		proxywasm.LogCriticalf("error retrieving data from queue %d: %v", queueID, err)
	}
}

func (ctx *servicePluginContext) onMessage(data []byte) {
	reqContext, err := shared.DecodeRequestContext(data)
	if err != nil {
		proxywasm.LogCriticalf("unable to decode data from queue: %v", err)
		return
	}

	if pausedHttpContextIDs, has := ctx.pausedClusters[reqContext.Authority]; has {
		proxywasm.LogDebugf("Adding httpContextID: %d for authority: %s", reqContext.HttpContextID, reqContext.Authority)
		ctx.pausedClusters[reqContext.Authority] = append(pausedHttpContextIDs, reqContext.HttpContextID)
	}
}

func (ctx *servicePluginContext) OnTick() {
	// Call our control plane to get the new list of scaled to zero clusters
	headers := [][2]string{
		{":method", "GET"},
		{":authority", "control-plane"},
		{":path", "/"},
		{"accept", "*/*"},
	}
	if _, err := proxywasm.DispatchHttpCall("control-plane", headers, nil, nil,
		5000, ctx.controlPlaneResponseCallback); err != nil {
		proxywasm.LogCriticalf("dipatch httpcall failed: %v", err)
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

	// 1) check for new clusters that are scaled to zero and add them to our internal state
	for _, authority := range currentScaledToZeroClusters {
		authority := authority
		if _, has := ctx.pausedClusters[authority]; !has {
			proxywasm.LogInfof("Adding %s to pausedClusters", authority)
			ctx.pausedClusters[authority] = make([]uint32, 0)
		}
	}

	// 2) update the shared state with all currently scaled to zero clusters
	proxywasm.LogInfof("Persisting %d paused clusters to the shared state", len(currentScaledToZeroClusters))
	clustersEncoded := shared.EncodeSharedData(currentScaledToZeroClusters)
	if err := proxywasm.SetSharedData(shared.ScaledToZeroClustersKey, clustersEncoded, 0); err != nil {
		proxywasm.LogCriticalf("error setting shared data: %v", err)
		return
	}

	// 3) remove scaled up clusters and unpause their requests
	for authority, _ := range ctx.pausedClusters {
		authority := authority

		if !slices.Contains(currentScaledToZeroClusters, authority) {
			proxywasm.LogInfof("Cluster %s is now scaled up and has %d pending http requests", authority, len(ctx.pausedClusters[authority]))

			// forward all pending requests
			for _, httpCtx := range ctx.pausedClusters[authority] {
				proxywasm.LogInfof("Resuming request with httpContextID: %d for cluster: %s", httpCtx, authority)
				err := proxywasm.SetEffectiveContext(httpCtx)
				if err != nil {
					proxywasm.LogCriticalf("failed to set http context: %v", err)
					return
				}
				err = proxywasm.ResumeHttpRequest()
				if err != nil {
					proxywasm.LogCriticalf("failed to resume request: %v", err)
					return
				}
			}

			proxywasm.LogInfof("removing %s from pausedClusters", authority)
			delete(ctx.pausedClusters, authority)
		}
	}
}
