package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

var ErrUnauthorized = errors.New("unauthorized")

// classifyGridError 把 /grid/generate 的错误映射为 HTTP 状态码 + 机器码 + 人话信息。
// 原始错误仅进日志，不回显，避免泄露内部细节。
func classifyGridError(err error) (status int, code, message string) {
	if err == nil {
		return http.StatusOK, CodeBadRequest, ""
	}
	if errors.Is(err, core.ErrRateLimited) {
		return http.StatusTooManyRequests, CodeRateLimited, "访问过于频繁"
	}
	if errors.Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized, CodeUnauthorized, "API Token 无效"
	}
	if core.IsGridImageRequestError(err) {
		return http.StatusBadRequest, CodeBadRequest, err.Error()
	}
	return http.StatusInternalServerError, CodeInternal, "内部错误"
}

// logInternalError 记录 5xx 级错误，附 request_id 便于排查。
func logInternalError(c *gin.Context, err error) {
	rid, _ := c.Get("request_id")
	core.Logger.Error("internal_error",
		"request_id", rid,
		"path", c.Request.URL.Path,
		"error", err.Error(),
	)
}
