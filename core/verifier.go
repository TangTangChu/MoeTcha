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

type VerifyResult struct {
	OK      bool   `json:"ok"`
	Reason  string `json:"reason,omitempty"`
	Correct int    `json:"correct"`
	Total   int    `json:"total"`
}

func VerifyGrid(chal *ChallengeInternal, req GridVerifyRequest) VerifyResult {
	if chal == nil || chal.Type != ChallengeGrid || chal.Grid == nil {
		return VerifyResult{OK: false, Reason: "challenge 类型不匹配"}
	}

	correctSet := make(map[string]struct{}, len(chal.Grid.CorrectImageIDs))
	for _, id := range chal.Grid.CorrectImageIDs {
		correctSet[id] = struct{}{}
	}

	if len(req.ImageIDs) == 0 {
		return VerifyResult{OK: false, Reason: "未选择图片", Total: len(correctSet)}
	}

	selected := make(map[string]struct{}, len(req.ImageIDs))
	for _, id := range req.ImageIDs {
		if id == "" {
			return VerifyResult{OK: false, Reason: "存在空图片ID", Total: len(correctSet)}
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
		return VerifyResult{OK: false, Reason: "数量不匹配", Correct: correct, Total: len(correctSet)}
	}

	if correct != len(correctSet) {
		return VerifyResult{OK: false, Reason: "包含错误选项", Correct: correct, Total: len(correctSet)}
	}

	return VerifyResult{OK: true, Correct: correct, Total: len(correctSet)}
}

func VerifyClick(chal *ChallengeInternal, req ClickVerifyRequest) VerifyResult {
	if chal == nil || chal.Type != ChallengeClick || chal.Click == nil {
		return VerifyResult{OK: false, Reason: "challenge 类型不匹配"}
	}

	regions := chal.Click.Regions
	if len(regions) == 0 {
		return VerifyResult{OK: false, Reason: "挑战缺少区域"}
	}

	if len(req.Points) == 0 {
		return VerifyResult{OK: false, Reason: "未提供点击点", Total: len(regions)}
	}

	matched := make(map[int]struct{})
	for _, p := range req.Points {
		idx := hitRegionIndex(regions, p)
		if idx < 0 {
			return VerifyResult{OK: false, Reason: fmt.Sprintf("点击点不在目标区域 x=%d y=%d", p.X, p.Y), Total: len(regions)}
		}
		matched[idx] = struct{}{}
	}

	if len(matched) != len(regions) {
		return VerifyResult{OK: false, Reason: "未命中所有区域", Correct: len(matched), Total: len(regions)}
	}

	return VerifyResult{OK: true, Correct: len(matched), Total: len(regions)}
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
