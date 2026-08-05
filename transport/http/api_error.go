package http

import (
	"errors"
	"net/http"
	"strings"

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
	if errors.Is(err, core.ErrServiceUninitialized) {
		return http.StatusInternalServerError, CodeServiceUninitialized, "服务组件未初始化"
	}
	if core.IsGridImageRequestError(err) {
		return http.StatusBadRequest, CodeBadRequest, err.Error()
	}
	return http.StatusInternalServerError, CodeInternal, "内部错误"
}

// classifyBindError 把 JSON 绑定错误映射为错误信封参数。
// 请求体超过 MaxBytesReader 上限时归 413 PAYLOAD_TOO_LARGE（文档错误码表），
// 其余解析错误归 400 BAD_REQUEST。
func classifyBindError(err error) (int, string, string) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "request body too large") {
		return http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "请求体超限"
	}
	return http.StatusBadRequest, CodeBadRequest, "请求 JSON 无效: " + err.Error()
}

// bindJSONBody 绑定 JSON 请求体；失败时已写入错误信封并返回 false。
func bindJSONBody(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		status, code, msg := classifyBindError(err)
		respondErr(c, status, code, msg)
		return false
	}
	return true
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
