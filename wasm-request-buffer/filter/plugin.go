package filter

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const tickMilliseconds uint32 = 1000

type pluginContext struct {
	types.DefaultPluginContext
	contextID      uint32
	pausedClusters map[string]struct{}
}

func (ctx *pluginContext) NewTcpContext(contextID uint32) types.TcpContext {
	return &networkContext{
		pluginCtx: ctx,
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
	// TODO: get this info from control-plane

	//for len(ctx.postponed) > 0 {
	//	httpCtxId, tail := ctx.postponed[0], ctx.postponed[1:]
	//	proxywasm.LogInfof("resume request with contextID=%v", httpCtxId)
	//	proxywasm.SetEffectiveContext(httpCtxId)
	//	proxywasm.Resume()
	//	ctx.postponed = tail
	//}
}
