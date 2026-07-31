package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

type challengeRequest struct {
	Type string `form:"type"`
}

func (r *Router) handleChallenge(c *gin.Context) {
	if r.Service == nil {
		respondErr(c, http.StatusInternalServerError, CodeServiceUninitialized, "service 未初始化")
		return
	}

	var req challengeRequest
	_ = c.ShouldBindQuery(&req)

	kind := core.ChallengeType(req.Type)
	ctx := core.VerifyContext{IP: clientIP(c), UserAgent: c.GetHeader("User-Agent"), RequestID: requestIDOf(c)}
	resp, err := r.Service.NewChallenge(kind, ctx)
	if err != nil {
		status, code, msg := classifyChallengeError(err)
		if status >= 500 {
			logInternalError(c, err)
		}
		respondErr(c, status, code, msg)
		return
	}

	respondOK(c, http.StatusOK, resp)
}

// classifyChallengeError 区分限流（429）与普通请求错误（400），
// 内部错误回落 500 但只回显通用信息。
func classifyChallengeError(err error) (status int, code, message string) {
	if errors.Is(err, core.ErrRateLimited) {
		return http.StatusTooManyRequests, CodeRateLimited, "访问过于频繁"
	}
	var ve *core.VerifyError
	if errors.As(err, &ve) {
		return verifyErrorStatus(ve.Code), ve.Code, ve.Message
	}
	return http.StatusBadRequest, CodeBadRequest, err.Error()
}
