package http

import (
"errors"
"net/http"

"github.com/gin-gonic/gin"
"moetcha/core"
)

var ErrUnauthorized = errors.New("unauthorized")

func respondAPIError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": message, "code": code})
}

// classifyGridError 将 /grid/generate 的错误映射为 HTTP 状态码与机器可读 code。
// internalErr 为原始错误，仅用于日志，不回显给客户端。
func classifyGridError(err error) (status int, code, publicMsg string) {
	if err == nil {
		return http.StatusOK, "OK", ""
	}
	if errors.Is(err, core.ErrRateLimited) {
		return http.StatusTooManyRequests, "RATE_LIMITED", "访问过于频繁"
	}
	if errors.Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized, "UNAUTHORIZED", "API Token 无效"
	}
	if core.IsGridImageRequestError(err) {
		return http.StatusBadRequest, "BAD_REQUEST", err.Error()
	}
	return http.StatusInternalServerError, "INTERNAL", "内部错误"
}
