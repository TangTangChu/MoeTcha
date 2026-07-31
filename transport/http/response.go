package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

// 通用 HTTP 错误码。请求级失败统一用这些。
const (
	CodeBadRequest            = "BAD_REQUEST"
	CodeUnauthorized          = "UNAUTHORIZED"
	CodeForbidden             = "FORBIDDEN"
	CodeNotFound              = "NOT_FOUND"
	CodeMethodNotAllowed      = "METHOD_NOT_ALLOWED"
	CodeRateLimited           = "RATE_LIMITED"
	CodePayloadTooLarge       = "PAYLOAD_TOO_LARGE"
	CodeInternal              = "INTERNAL"
	CodeServiceUninitialized  = "SERVICE_UNINITIALIZED"
)

// apiError 是信封里的 error 子对象。
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// 统一响应信封。所有 JSON 端点都走这个结构：
// 客户端只需判断 ok：true 读 data，false 读 error。
type apiResponse struct {
	OK        bool        `json:"ok"`
	Data      interface{} `json:"data,omitempty"`
	Error     *apiError   `json:"error,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// respondOK 写入成功信封：HTTP status + {ok:true, data, request_id}。
func respondOK(c *gin.Context, status int, data interface{}) {
	c.JSON(status, apiResponse{
		OK:        true,
		Data:      data,
		RequestID: requestIDOf(c),
		Timestamp: nowStamp(),
	})
}

// respondErr 写入失败信封：HTTP status + {ok:false, error:{code,message}, request_id}。
func respondErr(c *gin.Context, status int, code, message string) {
	c.JSON(status, apiResponse{
		OK:        false,
		Error:     &apiError{Code: code, Message: message},
		RequestID: requestIDOf(c),
		Timestamp: nowStamp(),
	})
}

func requestIDOf(c *gin.Context) string {
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// verifyErrorStatus 把 core.VerifyError 的请求级错误码映射到合适的 HTTP 状态。
// 验证码「答错」不在这里--那是 VerifyResult.Solved=false，走 200。
func verifyErrorStatus(code string) int {
	switch code {
	case core.CodeRateLimited:
		return http.StatusTooManyRequests
	case core.CodeTokenInvalid:
		return http.StatusUnauthorized
	case core.CodeIPMismatch, core.CodeUAMismatch, core.CodeMissingUA:
		return http.StatusForbidden
	case core.CodeSessionExpired:
		return http.StatusGone
	default:
		return http.StatusBadRequest
	}
}
