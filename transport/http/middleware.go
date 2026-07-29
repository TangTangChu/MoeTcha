package http

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = core.RandomHex(8)
		}
		c.Set("request_id", rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		rid, _ := c.Get("request_id")
		core.Logger.Info("request",
			"request_id", rid,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", duration.Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}

// APIAuthMiddleware 校验 Bearer Token 或 X-API-Token。
// tokens 为空时直接放行（仅适合内网/本地开发），非空时必须命中其中一个。
func APIAuthMiddleware(tokens []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		if t != "" {
			allowed[t] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.Next()
			return
		}
		token := extractBearerToken(c)
		if token == "" {
			respondAPIError(c, http.StatusUnauthorized, "UNAUTHORIZED", "缺少 API Token")
			return
		}
		if !tokenInSet(token, allowed) {
			respondAPIError(c, http.StatusUnauthorized, "UNAUTHORIZED", "API Token 无效")
			return
		}
		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-API-Token")); v != "" {
		return v
	}
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if auth == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return ""
	}
	return strings.TrimSpace(auth[len("bearer "):])
}

func tokenInSet(token string, allowed map[string]struct{}) bool {
	for t := range allowed {
		if subtle.ConstantTimeCompare([]byte(token), []byte(t)) == 1 {
			return true
		}
	}
	return false
}

