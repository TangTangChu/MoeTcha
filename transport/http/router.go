package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

type Router struct {
	Engine  *gin.Engine
	Service *core.Service
	Assets  core.AssetStore
	APIAuth core.APIAuthConfig
}

func NewRouter(service *core.Service, assets core.AssetStore, apiAuth core.APIAuthConfig) *Router {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestIDMiddleware())
	r.Use(LoggingMiddleware())

	router := &Router{Engine: r, Service: service, Assets: assets, APIAuth: apiAuth}
	router.registerRoutes()
	return router
}

// SetReleaseMode 切换 gin 为发布模式，抑制启动时那一串 [GIN-debug] 路由
// 注册输出与 "Listening and serving" 行，交由调用方自行打印干净的就绪信息。
func SetReleaseMode() {
	gin.SetMode(gin.ReleaseMode)
}

func (r *Router) registerRoutes() {
	r.Engine.GET("/challenge", r.handleChallenge)
	r.Engine.POST("/verify", r.handleVerify)
	r.Engine.POST("/grid/generate", APIAuthMiddleware(r.APIAuth.Tokens), r.handleGridGenerate)
	r.Engine.GET("/asset/:key", r.handleAsset)
	r.Engine.Any("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": http.StatusOK,
			"text":   "Ciallo～(∠・ω< )⌒★",
			"method": c.Request.Method,
		})
	})
	r.Engine.GET("/metrics", func(c *gin.Context) {
		c.JSON(http.StatusOK, core.MetricsInstance.Snapshot())
	})
}
