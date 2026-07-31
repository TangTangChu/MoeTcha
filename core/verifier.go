package core

import (
	"fmt"
	"sort"
)

type GridVerifyRequest struct {
	ImageIDs []string `json:"image_ids"`
}

type ClickPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type ClickVerifyRequest struct {
	Points []ClickPoint `json:"points"`
}

// VerifyResult 是验证码本身的判定结果，与「请求是否可处理」无关。
// 请求级失败（会话过期、限流等）走 VerifyError，不会进入这里。
type VerifyResult struct {
	Solved  bool   `json:"solved"`
	Code    string `json:"code,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Correct int    `json:"correct"`
	Total   int    `json:"total"`
}

// 请求级失败码。Verify 拿到这些情形时返回 VerifyError，
// handler 据此映射 HTTP 状态码与回显信息。
const (
	CodeEmptySession     = "EMPTY_SESSION"
	CodeSessionExpired   = "SESSION_EXPIRED"
	CodeTooFast          = "TOO_FAST"
	CodeIPMismatch       = "IP_MISMATCH"
	CodeMissingUA        = "MISSING_UA"
	CodeUAMismatch       = "UA_MISMATCH"
	CodeTooManyAttempts  = "TOO_MANY_ATTEMPTS"
	CodeHighFailRatio    = "HIGH_FAIL_RATIO"
	CodeRateLimited      = "RATE_LIMITED"
	CodeTokenInvalid     = "TOKEN_INVALID"
	CodeChallengeMissing = "CHALLENGE_MISSING"
	CodeMissingGrid      = "MISSING_GRID"
	CodeMissingClick     = "MISSING_CLICK"
	CodeUnknownType      = "UNKNOWN_TYPE"
)

// 验证判定码。描述「验证码为何没通过」，便于前端 i18n 与程序化处理。
const (
	CodeTypeMismatch     = "TYPE_MISMATCH"
	CodeNoSelection      = "NO_SELECTION"
	CodeEmptyImageID     = "EMPTY_IMAGE_ID"
	CodeWrongCount       = "WRONG_COUNT"
	CodeWrongSelection   = "WRONG_SELECTION"
	CodeNoRegions        = "NO_REGIONS"
	CodeNoPoints         = "NO_POINTS"
	CodePointOutOfRegion = "POINT_OUT_OF_REGION"
	CodeWrongClickCount  = "WRONG_CLICK_COUNT"
	CodeDuplicateRegion  = "DUPLICATE_REGION"
)

// VerifyError 表示请求级失败：会话无效、被限流、绑定校验不过等。
// 与 VerifyResult 区分——前者意味着请求根本没能进入判定，后者是判定完成。
type VerifyError struct {
	Code    string
	Message string
}

func (e *VerifyError) Error() string { return e.Message }

func NewVerifyError(code, message string) *VerifyError {
	return &VerifyError{Code: code, Message: message}
}

func VerifyGrid(chal *ChallengeInternal, req GridVerifyRequest) VerifyResult {
	if chal == nil || chal.Type != ChallengeGrid || chal.Grid == nil {
		return VerifyResult{Solved: false, Code: CodeTypeMismatch, Reason: "challenge 类型不匹配"}
	}

	correctSet := make(map[string]struct{}, len(chal.Grid.CorrectImageIDs))
	for _, id := range chal.Grid.CorrectImageIDs {
		correctSet[id] = struct{}{}
	}

	if len(req.ImageIDs) == 0 {
		return VerifyResult{Solved: false, Code: CodeNoSelection, Reason: "未选择图片", Total: len(correctSet)}
	}

	selected := make(map[string]struct{}, len(req.ImageIDs))
	for _, id := range req.ImageIDs {
		if id == "" {
			return VerifyResult{Solved: false, Code: CodeEmptyImageID, Reason: "存在空图片ID", Total: len(correctSet)}
		}
		selected[id] = struct{}{}
	}

	correct := 0
	for id := range selected {
		if _, ok := correctSet[id]; ok {
			correct++
		}
	}

	if len(selected) != len(correctSet) {
		return VerifyResult{Solved: false, Code: CodeWrongCount, Reason: "数量不匹配", Correct: correct, Total: len(correctSet)}
	}

	if correct != len(correctSet) {
		return VerifyResult{Solved: false, Code: CodeWrongSelection, Reason: "包含错误选项", Correct: correct, Total: len(correctSet)}
	}

	return VerifyResult{Solved: true, Correct: correct, Total: len(correctSet)}
}

func VerifyClick(chal *ChallengeInternal, req ClickVerifyRequest) VerifyResult {
	if chal == nil || chal.Type != ChallengeClick || chal.Click == nil {
		return VerifyResult{Solved: false, Code: CodeTypeMismatch, Reason: "challenge 类型不匹配"}
	}

	regions := chal.Click.Regions
	if len(regions) == 0 {
		return VerifyResult{Solved: false, Code: CodeNoRegions, Reason: "挑战缺少区域"}
	}

	// Required 为 0（含旧数据）时退化为「点击全部」。
	required := chal.Click.Required
	if required <= 0 {
		required = len(regions)
	}

	if len(req.Points) == 0 {
		return VerifyResult{Solved: false, Code: CodeNoPoints, Reason: "未提供点击点", Total: required}
	}

	matched := make(map[int]struct{})
	for _, p := range req.Points {
		idx := hitRegionIndex(regions, p)
		if idx < 0 {
			return VerifyResult{Solved: false, Code: CodePointOutOfRegion, Reason: fmt.Sprintf("点击点不在目标区域 x=%d y=%d", p.X, p.Y), Total: required}
		}
		matched[idx] = struct{}{}
	}

	if len(req.Points) != required {
		return VerifyResult{Solved: false, Code: CodeWrongClickCount, Reason: "点击数量不匹配", Correct: len(matched), Total: required}
	}

	if len(matched) != len(req.Points) {
		return VerifyResult{Solved: false, Code: CodeDuplicateRegion, Reason: "存在重复点击的区域", Correct: len(matched), Total: required}
	}

	return VerifyResult{Solved: true, Correct: len(matched), Total: required}
}

func hitRegionIndex(regions []Region, p ClickPoint) int {
	for i, r := range regions {
		if pointInRegion(p, r) {
			return i
		}
	}
	return -1
}

func pointInRegion(p ClickPoint, r Region) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X <= r.X+r.Width && p.Y <= r.Y+r.Height
}

func SortedStringSlice(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}
