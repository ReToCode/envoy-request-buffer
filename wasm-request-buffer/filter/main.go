package main

import (
	"slices"

	"github.com/retocode/envoy-request-buffer/wasm-request-buffer/shared"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const hostHeaderKey = "host"

const tickMilliseconds uint32 = 1000 // every second

type filterVmContext struct {
	types.DefaultVMContext
}

type filterPluginContext struct {
	types.DefaultPluginContext
	contextID                uint32
	config                   *shared.PluginConfig
	pausedRequestsForCluster map[string][]uint32 // [host][[]httpContextIDs]
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
	for host, pendingHTTPContexts := range ctx.pausedRequestsForCluster {
		if !slices.Contains(scaledToZeroClusters, host) {
			proxywasm.LogInfof("%s is no longer scaled to zero and has %d pending http requests", host, len(pendingHTTPContexts))

			// forward all pending requests
			for _, httpCtx := range pendingHTTPContexts {
				proxywasm.LogInfof("Resuming request with ctx: %d for cluster: %s", httpCtx, host)
				err := proxywasm.SetEffectiveContext(httpCtx)
				if err != nil {
					// error can happen when client already the connection
					proxywasm.LogDebugf("failed to set http context: %v", err)
				}
				err = proxywasm.ResumeHttpRequest()
				if err != nil {
					// error can happen when client already the connection
					proxywasm.LogDebugf("failed to resume request: %v", err)
				}
			}

			proxywasm.LogDebugf("Removing %s from pausedRequestsForCluster", host)
			delete(ctx.pausedRequestsForCluster, host)
		}
	}
}

func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	host, err := proxywasm.GetHttpRequestHeader(hostHeaderKey)
	if err != nil {
		proxywasm.LogCritical("failed to get request http header: host'")
		return types.ActionContinue
	}

	// Check on shared data if current target is scaled to zero
	scaledToZeroClusters, err := getScaledToZeroClusters()
	if err != nil {
		proxywasm.LogCriticalf("failed to get scaled to zero state: %v", err)
		return types.ActionContinue
	}
	if slices.Contains(scaledToZeroClusters, host) {
		proxywasm.LogDebugf("%s is scaled to zero, pausing http request with httpContextID: %d", host, ctx.httpContextID)

		pendingRequests, has := ctx.pluginCtx.pausedRequestsForCluster[host]
		if has {
			ctx.pluginCtx.pausedRequestsForCluster[host] = append(pendingRequests, ctx.httpContextID)
		} else {
			ctx.pluginCtx.pausedRequestsForCluster[host] = []uint32{ctx.httpContextID}
		}

		// TODO: we could optimize this
		// 1) debounce it
		// 2) do it from the shared service using a queue
		proxywasm.LogDebugf("Poking scale-up for host: %s on http request with httpContextID: %d", host, ctx.httpContextID)
		headers := [][2]string{
			{":method", "POST"},
			{":authority", ctx.pluginCtx.config.ControlPlaneURL},
			{":path", "/poke-scale-up?host=" + host},
			{"accept", "*/*"},
		}

		proxywasm.LogInfof("Calling out to %s with headers: %v", ctx.pluginCtx.config.ControlPlaneCluster, headers)

		if _, err := proxywasm.DispatchHttpCall(ctx.pluginCtx.config.ControlPlaneCluster, headers, nil, nil,
			5000, func(numHeaders, bodySize, numTrailers int) {
				// we just log the response here
				headers, err := proxywasm.GetHttpCallResponseHeaders()
				if err != nil {
					proxywasm.LogCriticalf("failed to get control-plane response headers: %v", err)
					return
				}

				proxywasm.LogInfof("Received the following response headers from control-plane: %s", headers)
			}); err != nil {
			proxywasm.LogCriticalf("dispatch httpcall failed: %v", err)
		}

		return types.ActionPause
	}

	proxywasm.LogDebugf("Service is scaled up, directly forwarding http request to %s", host)
	return types.ActionContinue
}

func getScaledToZeroClusters() ([]string, error) {
	data, _, err := proxywasm.GetSharedData(shared.ScaledToZeroClustersKey)
	if err != nil {
		return nil, err
	}

	return shared.DecodeSharedData(data), nil
}
