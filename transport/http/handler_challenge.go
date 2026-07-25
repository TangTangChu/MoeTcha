package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

type challengeRequest struct {
	Type string `form:"type"`
}

func (r *Router) handleChallenge(c *gin.Context) {
	if r.Service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "service 未初始化"})
		return
	}

	var req challengeRequest
	_ = c.ShouldBindQuery(&req)

	kind := core.ChallengeType(req.Type)
	ctx := core.VerifyContext{IP: clientIP(c), UserAgent: c.GetHeader("User-Agent")}
	resp, err := r.Service.NewChallenge(kind, ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
