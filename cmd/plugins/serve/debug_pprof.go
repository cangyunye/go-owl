package serve

import (
	"net/http/pprof"

	"github.com/gin-gonic/gin"
)

// registerPprofRoutes 在 admin 路由组（登录 + RoleAdmin）下暴露 pprof 端点：
// /api/v1/debug/pprof/{heap,allocs,goroutine,block,mutex,threadcreate,
// profile,trace,cmdline,symbol}。逐个显式注册而非通配转发，因为
// pprof.Index 只按 "/debug/pprof/" 前缀解析子路径，挂在 /api/v1 下会失配。
//
// 远程抓取（需要 JWT，go tool pprof 不便携带 Bearer 头）：
//
//	curl -H "Authorization: Bearer $TOKEN" \
//	  http://127.0.0.1:8080/api/v1/debug/pprof/heap > heap.pb.gz
//	go tool pprof -top heap.pb.gz
func registerPprofRoutes(rg *gin.RouterGroup) {
	rg.GET("/debug/pprof", gin.WrapF(pprof.Index))
	rg.GET("/debug/pprof/", gin.WrapF(pprof.Index))
	rg.GET("/debug/pprof/cmdline", gin.WrapF(pprof.Cmdline))
	rg.GET("/debug/pprof/profile", gin.WrapF(pprof.Profile))
	rg.GET("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	rg.POST("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	rg.GET("/debug/pprof/trace", gin.WrapF(pprof.Trace))
	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		rg.GET("/debug/pprof/"+name, gin.WrapH(pprof.Handler(name)))
	}
}
