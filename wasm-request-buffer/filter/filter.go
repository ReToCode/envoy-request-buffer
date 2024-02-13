package filter

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const authorityKey = ":authority"

type httpContext struct {
	types.DefaultHttpContext
	pluginCtx *pluginContext
	contextID uint32
}

func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	authority, err := proxywasm.GetHttpRequestHeader(authorityKey)
	if err != nil {
		proxywasm.LogCritical("failed to get request header: ':authority'")
		return types.ActionPause
	}

	if pendingRequests, has := ctx.pluginCtx.scaledToZeroClusters[authority]; has {
		proxywasm.LogInfof("Http request to %s are paused, ctxId: %d", authority, ctx.contextID)
		ctx.pluginCtx.scaledToZeroClusters[authority] = append(pendingRequests, ctx.contextID)
		return types.ActionPause
	}

	proxywasm.LogInfof("Service is scaled up, directly forwarding http request to %s", authority)
	return types.ActionContinue
}
