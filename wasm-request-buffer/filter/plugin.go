package filter

import (
	"encoding/json"
	"slices"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const tickMilliseconds uint32 = 1000 * 10 // every 5 seconds

type pluginContext struct {
	types.DefaultPluginContext
	contextID uint32

	// [hostname]httpContextID
	scaledToZeroClusters map[string][]uint32
}

func (ctx *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpContext{
		pluginCtx: ctx,
		contextID: contextID,
	}
}

func (ctx *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	if err := proxywasm.SetTickPeriodMilliSeconds(tickMilliseconds); err != nil {
		proxywasm.LogCriticalf("failed to set tick period: %v", err)
	}

	proxywasm.LogInfo("Plugin started with ticker")

	return types.OnPluginStartStatusOK
}

func (ctx *pluginContext) OnTick() {
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

func (ctx *pluginContext) controlPlaneResponseCallback(numHeaders, bodySize, numTrailers int) {
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

	// add new scaled to zero clusters
	for _, s := range currentScaledToZeroClusters {
		s := s
		if _, has := ctx.scaledToZeroClusters[s]; !has {
			proxywasm.LogInfof("adding %s to scaledToZeroClusters", s)
			ctx.scaledToZeroClusters[s] = make([]uint32, 0)
		}
	}

	proxywasm.LogInfof("cluster has the following pending http contexts: %v", ctx.scaledToZeroClusters)

	// remove existing ones that are now scaled up
	for s, _ := range ctx.scaledToZeroClusters {
		s := s

		if !slices.Contains(currentScaledToZeroClusters, s) {

			proxywasm.LogInfof("specific has the following pending http contexts: %v", ctx.scaledToZeroClusters[s])

			// forward all pending requests
			for _, httpCtx := range ctx.scaledToZeroClusters[s] {
				proxywasm.LogInfof("resuming request with ctx: %d for cluster: %s", httpCtx, s)
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

				// todo: remove the context ID from the slice
			}

			proxywasm.LogInfof("removing %s to scaledToZeroClusters", s)
			delete(ctx.scaledToZeroClusters, s)
		}
	}
}
