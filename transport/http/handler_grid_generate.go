package http

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

func (r *Router) handleGridGenerate(c *gin.Context) {
	if r.Service == nil || r.Assets == nil {
		respondErr(c, http.StatusInternalServerError, CodeServiceUninitialized, "service 或 asset store 未初始化")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req core.GridImageGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, CodeBadRequest, "请求 JSON 无效: "+err.Error())
		return
	}

	result, err := r.Service.GenerateGridImage(req, core.VerifyContext{
		IP:        resolveClientIP(c, r.IPResolve),
		UserAgent: c.GetHeader("User-Agent"),
		RequestID: requestIDOf(c),
	})
	if err != nil {
		status, code, msg := classifyGridError(err)
		if status >= 500 {
			logInternalError(c, err)
		}
		respondErr(c, status, code, msg)
		return
	}

	assetPath := "/asset/" + url.PathEscape(result.AssetKey)
	result.AssetURL = absoluteRequestURL(c, assetPath)
	result.TemporaryFileURL = result.AssetURL
	respondOK(c, http.StatusOK, result)
}

func absoluteRequestURL(c *gin.Context, path string) string {
	if c == nil || c.Request == nil || c.Request.Host == "" {
		return path
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if forwarded := c.GetHeader("X-Forwarded-Proto"); forwarded != "" {
		candidate := strings.ToLower(strings.TrimSpace(strings.Split(forwarded, ",")[0]))
		if candidate == "http" || candidate == "https" {
			scheme = candidate
		}
	}
	return scheme + "://" + c.Request.Host + path
}
