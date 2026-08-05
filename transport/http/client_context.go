package http

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

// resolveClientIP 按策略提取客户端 IP。头部缺失或非法时回落直连 IP。
func resolveClientIP(c *gin.Context, cfg core.IPResolveConfig) string {
	switch cfg.Source {
	case "x-forwarded-for":
		if ip := pickXFF(c.GetHeader("X-Forwarded-For"), cfg.XFFIndex); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	case "x-real-ip":
		if ip := strings.TrimSpace(c.GetHeader("X-Real-IP")); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}
	return remotePeerIP(c)
}

// pickXFF 取 X-Forwarded-For 第 idx 个非空 IP。idx<0 从右数。
func pickXFF(xff string, idx int) string {
	var ips []string
	for _, p := range strings.Split(xff, ",") {
		if s := strings.TrimSpace(p); s != "" {
			ips = append(ips, s)
		}
	}
	if len(ips) == 0 {
		return ""
	}
	i := idx
	if i < 0 {
		i = len(ips) + i
	}
	if i < 0 || i >= len(ips) {
		return ""
	}
	return ips[i]
}

// remotePeerIP 返回 TCP 直连对端 IP。
func remotePeerIP(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
