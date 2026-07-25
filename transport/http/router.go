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
}

func NewRouter(service *core.Service, assets core.AssetStore) *Router {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestIDMiddleware())
	r.Use(LoggingMiddleware())

	router := &Router{Engine: r, Service: service, Assets: assets}
	router.registerRoutes()
	return router
}

func (r *Router) registerRoutes() {
	r.Engine.GET("/challenge", r.handleChallenge)
	r.Engine.POST("/verify", r.handleVerify)
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
