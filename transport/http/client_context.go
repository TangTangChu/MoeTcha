package http

import "github.com/gin-gonic/gin"

func clientIP(c *gin.Context) string {
	ip := c.ClientIP()
	return ip
}
