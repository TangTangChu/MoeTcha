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
		core.Logger.Info("request",
			"request_id", requestIDOf(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}

// SecurityHeadersMiddleware 注入基础安全响应头。
// nosniff 防止 asset 端点被浏览器按别的内容类型嗅探，
// DENY 防止页面被嵌套进 iframe 做点击劫持。
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		c.Next()
	}
}

// CORSMiddleware 处理跨域预检与响应头。
// AllowedOrigins 含 "*" 时放行任意来源；否则按精确匹配回显 Origin。
func CORSMiddleware(cfg core.CORSConfig) gin.HandlerFunc {
	allowAll := false
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			allowAll = true
		}
		allowed[o] = struct{}{}
	}
	enabled := cfg.Enabled || allowAll || len(allowed) > 0

	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		h := c.Writer.Header()

		switch {
		case allowAll:
			h.Set("Access-Control-Allow-Origin", "*")
		case origin != "":
			if _, ok := allowed[origin]; ok {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
			}
		}

		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Token, X-Request-ID")
		// X-Request-ID 由 RequestIDMiddleware 写入响应头，供客户端排查关联。
		// 跨域浏览器默认只能读到“简单响应头”，必须显式暴露才能被 fetch 读取。
		h.Set("Access-Control-Expose-Headers", "X-Request-ID")
		h.Set("Access-Control-Max-Age", "600")

		// 预检直接短路，避免命中下游 NoRoute 的 404。
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// APIAuthMiddleware 校验 Bearer Token 或 X-API-Token。
// tokens 为空时放行（仅适合内网/本地开发），非空时必须命中其一。
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
			respondErr(c, http.StatusUnauthorized, CodeUnauthorized, "缺少 API Token")
			c.Abort()
			return
		}
		if !tokenInSet(token, allowed) {
			respondErr(c, http.StatusUnauthorized, CodeUnauthorized, "API Token 无效")
			c.Abort()
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
