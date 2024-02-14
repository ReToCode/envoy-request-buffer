package main

import (
	"slices"

	"github.com/retocode/envoy-request-buffer/wasm-request-buffer/shared"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const authorityKey = ":authority"

type filterVmContext struct {
	types.DefaultVMContext
}

type filterPluginContext struct {
	types.DefaultPluginContext
	contextID uint32
}

type httpContext struct {
	types.DefaultHttpContext
	pluginCtx          *filterPluginContext
	httpContextID      uint32
	httpContextQueueID uint32
}

func main() {
	proxywasm.SetVMContext(&filterVmContext{})
}

func (*filterVmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &filterPluginContext{
		contextID: contextID,
	}
}

func (ctx *filterPluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	httpContextQueueID, err := proxywasm.ResolveSharedQueue(shared.VMID, shared.HTTPContextQueueName)
	if err != nil {
		proxywasm.LogCriticalf("error resolving queue id: %v", err)
	}

	return &httpContext{
		pluginCtx:          ctx,
		httpContextID:      contextID,
		httpContextQueueID: httpContextQueueID,
	}
}

func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	authority, err := proxywasm.GetHttpRequestHeader(authorityKey)
	if err != nil {
		proxywasm.LogCritical("failed to get request header: ':authority'")
		return types.ActionContinue
	}

	// Check on shared data if current target is scaled to zero
	scaledToZero, err := ctx.isScaledToZero(authority)
	if err != nil {
		proxywasm.LogCriticalf("failed to get scaled to zero state: %v", err)
		return types.ActionContinue
	}
	if scaledToZero {
		proxywasm.LogInfof("%s is scaled to zero, http request will be paused, httpContextID: %d", authority, ctx.httpContextID)

		payload := shared.EncodeRequestContext(shared.RequestContext{
			Authority:     authority,
			HttpContextID: ctx.httpContextID,
		})
		if err := proxywasm.EnqueueSharedQueue(ctx.httpContextQueueID, payload); err != nil {
			proxywasm.LogCriticalf("failed to send httpContext to queue: %v", err)
			return types.ActionContinue
		}

		proxywasm.LogInfof("sent httpContextID: %d to queue", ctx.httpContextID)
		return types.ActionPause
	}

	proxywasm.LogInfof("Service is scaled up, directly forwarding http request to %s", authority)
	return types.ActionContinue
}

func (ctx *httpContext) isScaledToZero(authority string) (bool, error) {
	data, _, err := proxywasm.GetSharedData(shared.ScaledToZeroClustersKey)
	if err != nil {
		proxywasm.LogCriticalf("error getting shared data: %v", err)
		return false, err
	}

	scaledToZeroClusters := shared.DecodeSharedData(data)
	isScaledToZero := slices.Contains(scaledToZeroClusters, authority)
	proxywasm.LogDebugf("%s isScaledToZero: %v", authority, isScaledToZero)
	return isScaledToZero, nil
}
