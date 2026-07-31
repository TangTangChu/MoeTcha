package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
	"moetcha/core/render"
)

func (r *Router) handleAsset(c *gin.Context) {
	if r.Assets == nil {
		respondErr(c, http.StatusInternalServerError, CodeServiceUninitialized, "asset store 未初始化")
		return
	}

	key := c.Param("key")
	if key == "" {
		respondErr(c, http.StatusBadRequest, CodeBadRequest, "asset key 为空")
		return
	}

	asset, ok := r.Assets.Get(key)
	if !ok || len(asset.Bytes) == 0 {
		respondErr(c, http.StatusNotFound, CodeNotFound, "asset 不存在或已过期")
		return
	}

	core.MetricsInstance.AssetsServed.Add(1)
	c.Data(http.StatusOK, render.ContentTypeForBytes(asset.Bytes), asset.Bytes)
}
