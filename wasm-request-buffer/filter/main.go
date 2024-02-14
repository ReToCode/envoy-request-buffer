package main

import (
	"slices"

	"github.com/retocode/envoy-request-buffer/wasm-request-buffer/shared"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const authorityKey = ":authority"

const tickMilliseconds uint32 = 1000 * 1 // every second

type filterVmContext struct {
	types.DefaultVMContext
}

type filterPluginContext struct {
	types.DefaultPluginContext
	contextID                uint32
	pausedRequestsForCluster map[string][]uint32 // [authority][[]httpContextIDs]
}

type httpContext struct {
	types.DefaultHttpContext
	pluginCtx     *filterPluginContext
	httpContextID uint32
}

func main() {
	proxywasm.SetVMContext(&filterVmContext{})
}

func (*filterVmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &filterPluginContext{
		contextID:                contextID,
		pausedRequestsForCluster: make(map[string][]uint32),
	}
}

func (ctx *filterPluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	if err := proxywasm.SetTickPeriodMilliSeconds(tickMilliseconds); err != nil {
		proxywasm.LogCriticalf("failed to set tick period: %v", err)
	}

	proxywasm.LogInfo("Filter plugin started with ticker")

	return types.OnPluginStartStatusOK
}

func (ctx *filterPluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpContext{
		pluginCtx:     ctx,
		httpContextID: contextID,
	}
}

func (ctx *filterPluginContext) OnTick() {
	scaledToZeroClusters, err := getScaledToZeroClusters()
	if err != nil {
		proxywasm.LogCriticalf("failed to get scaled to zero state: %v", err)
		return
	}

	// check which clusters are no longer scaled to zero
	for authority, pendingHTTPContexts := range ctx.pausedRequestsForCluster {
		if !slices.Contains(scaledToZeroClusters, authority) {
			proxywasm.LogInfof("%s is no longer scaled to zero and has %d pending http requests", authority, len(pendingHTTPContexts))

			// forward all pending requests
			for _, httpCtx := range pendingHTTPContexts {
				proxywasm.LogInfof("Resuming request with ctx: %d for cluster: %s", httpCtx, authority)
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

			proxywasm.LogDebugf("Removing %s from pausedRequestsForCluster", authority)
			delete(ctx.pausedRequestsForCluster, authority)
		}
	}
}

func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	authority, err := proxywasm.GetHttpRequestHeader(authorityKey)
	if err != nil {
		proxywasm.LogCritical("failed to get request header: ':authority'")
		return types.ActionContinue
	}

	// Check on shared data if current target is scaled to zero
	scaledToZeroClusters, err := getScaledToZeroClusters()
	if err != nil {
		proxywasm.LogCriticalf("failed to get scaled to zero state: %v", err)
		return types.ActionContinue
	}
	if slices.Contains(scaledToZeroClusters, authority) {
		proxywasm.LogDebugf("%s is scaled to zero, pausing http request with httpContextID: %d", authority, ctx.httpContextID)

		pendingRequests, has := ctx.pluginCtx.pausedRequestsForCluster[authority]
		if has {
			ctx.pluginCtx.pausedRequestsForCluster[authority] = append(pendingRequests, ctx.httpContextID)
		} else {
			ctx.pluginCtx.pausedRequestsForCluster[authority] = []uint32{ctx.httpContextID}
		}

		return types.ActionPause
	}

	proxywasm.LogDebugf("Service is scaled up, directly forwarding http request to %s", authority)
	return types.ActionContinue
}

func getScaledToZeroClusters() ([]string, error) {
	data, _, err := proxywasm.GetSharedData(shared.ScaledToZeroClustersKey)
	if err != nil {
		return nil, err
	}

	return shared.DecodeSharedData(data), nil
}
