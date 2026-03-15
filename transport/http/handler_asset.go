package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core/render"
)

func (r *Router) handleAsset(c *gin.Context) {
	if r.Assets == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "asset store 未初始化"})
		return
	}

	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "asset key 为空"})
		return
	}

	asset, ok := r.Assets.Get(key)
	if !ok || len(asset.Bytes) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset 不存在"})
		return
	}

	c.Data(http.StatusOK, render.ContentType(), asset.Bytes)
}
