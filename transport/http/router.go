package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

type RouterConfig struct {
	APIAuth core.APIAuthConfig
	CORS    core.CORSConfig
}

type Router struct {
	Engine  *gin.Engine
	Service *core.Service
	Assets  core.AssetStore
	APIAuth core.APIAuthConfig
	CORS    core.CORSConfig
}

func NewRouter(service *core.Service, assets core.AssetStore, cfg RouterConfig) *Router {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestIDMiddleware())
	r.Use(SecurityHeadersMiddleware())
	r.Use(CORSMiddleware(cfg.CORS))
	r.Use(LoggingMiddleware())

	router := &Router{Engine: r, Service: service, Assets: assets, APIAuth: cfg.APIAuth, CORS: cfg.CORS}
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
		respondOK(c, http.StatusOK, gin.H{
			"status": http.StatusOK,
			"text":   "Ciallo～(∠・ω< )⌒★",
			"method": c.Request.Method,
		})
	})
	r.Engine.GET("/metrics", APIAuthMiddleware(r.APIAuth.Tokens), func(c *gin.Context) {
		respondOK(c, http.StatusOK, core.MetricsInstance.Snapshot())
	})

	// 未匹配路由统一返回 404 信封，而不是 gin 默认的裸 404。
	r.Engine.NoRoute(func(c *gin.Context) {
		respondErr(c, http.StatusNotFound, CodeNotFound, "请求的资源不存在："+c.Request.URL.Path)
	})
	r.Engine.NoMethod(func(c *gin.Context) {
		respondErr(c, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "不支持的请求方法："+c.Request.Method)
	})
	r.Engine.HandleMethodNotAllowed = true
}
