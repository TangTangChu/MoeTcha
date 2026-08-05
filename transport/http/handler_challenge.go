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
	ctx := core.VerifyContext{IP: resolveClientIP(c, r.IPResolve), UserAgent: c.GetHeader("User-Agent"), RequestID: requestIDOf(c)}
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
// 未知挑战类型是唯一的客户端参数错误；组件未初始化归 SERVICE_UNINITIALIZED；
// 其余（素材缺失、无可用标签等）是服务端状态问题，归 500 INTERNAL 不回显细节。
func classifyChallengeError(err error) (status int, code, message string) {
	if errors.Is(err, core.ErrRateLimited) {
		return http.StatusTooManyRequests, CodeRateLimited, "访问过于频繁"
	}
	var ve *core.VerifyError
	if errors.As(err, &ve) {
		return verifyErrorStatus(ve.Code), ve.Code, ve.Message
	}
	if errors.Is(err, core.ErrUnknownChallengeType) {
		return http.StatusBadRequest, CodeBadRequest, err.Error()
	}
	if errors.Is(err, core.ErrServiceUninitialized) {
		return http.StatusInternalServerError, CodeServiceUninitialized, "服务组件未初始化"
	}
	return http.StatusInternalServerError, CodeInternal, "内部错误"
}
