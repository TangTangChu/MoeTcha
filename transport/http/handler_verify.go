package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

type verifyRequest struct {
	SessionID string                   `json:"session_id"`
	Token     string                   `json:"token"`
	Grid      *core.GridVerifyRequest  `json:"grid,omitempty"`
	Click     *core.ClickVerifyRequest `json:"click,omitempty"`
}

func (r *Router) handleVerify(c *gin.Context) {
	if r.Service == nil {
		respondErr(c, http.StatusInternalServerError, CodeServiceUninitialized, "service 未初始化")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req verifyRequest
	if !bindJSONBody(c, &req) {
		return
	}

	ctx := core.VerifyContext{
		IP:        resolveClientIP(c, r.IPResolve),
		UserAgent: c.GetHeader("User-Agent"),
		Token:     req.Token,
		RequestID: requestIDOf(c),
	}
	result, err := r.Service.Verify(req.SessionID, req.Grid, req.Click, ctx)
	if err != nil {
		// 请求级失败：会话过期、限流、绑定校验不过等。
		var ve *core.VerifyError
		if errors.As(err, &ve) {
			respondErr(c, verifyErrorStatus(ve.Code), ve.Code, ve.Message)
			return
		}
		// 走到这里说明是非预期错误，不回显细节。
		logInternalError(c, err)
		respondErr(c, http.StatusInternalServerError, CodeInternal, "内部错误")
		return
	}

	// 验证码判定完成：无论通过与否都返回 200，
	// 客户端读 data.solved 区分，而非靠 HTTP 状态码猜。
	respondOK(c, http.StatusOK, result)
}
