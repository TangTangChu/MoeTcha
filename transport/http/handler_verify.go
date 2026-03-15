package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

type verifyRequest struct {
	SessionID string                   `json:"session_id"`
	Type      string                   `json:"type"`
	Grid      *core.GridVerifyRequest  `json:"grid,omitempty"`
	Click     *core.ClickVerifyRequest `json:"click,omitempty"`
}

func (r *Router) handleVerify(c *gin.Context) {
	if r.Service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "service 未初始化"})
		return
	}

	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := r.Service.Verify(req.SessionID, req.Grid, req.Click)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	status := http.StatusOK
	if !result.OK {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}
