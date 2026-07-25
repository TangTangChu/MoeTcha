package core

import "testing"

func TestVerifyGrid(t *testing.T) {
	chal := &ChallengeInternal{
		Type: ChallengeGrid,
		Grid: &GridChallengeInternal{
			CorrectImageIDs: []string{"a:cat1", "a:cat2"},
		},
	}

	tests := []struct {
		name    string
		req     GridVerifyRequest
		wantOK  bool
		wantCor int
		wantTot int
	}{
		{
			name:    "完全匹配",
			req:     GridVerifyRequest{ImageIDs: []string{"a:cat1", "a:cat2"}},
			wantOK:  true,
			wantCor: 2,
			wantTot: 2,
		},
		{
			name:    "顺序不同也能匹配",
			req:     GridVerifyRequest{ImageIDs: []string{"a:cat2", "a:cat1"}},
			wantOK:  true,
			wantCor: 2,
			wantTot: 2,
		},
		{
			name:    "数量不足",
			req:     GridVerifyRequest{ImageIDs: []string{"a:cat1"}},
			wantOK:  false,
			wantCor: 1,
			wantTot: 2,
		},
		{
			name:    "数量过多且包含正确",
			req:     GridVerifyRequest{ImageIDs: []string{"a:cat1", "a:cat2", "a:dog1"}},
			wantOK:  false,
			wantCor: 2,
			wantTot: 2,
		},
		{
			name:    "全错",
			req:     GridVerifyRequest{ImageIDs: []string{"a:dog1", "a:dog2"}},
			wantOK:  false,
			wantCor: 0,
			wantTot: 2,
		},
		{
			name:    "空输入",
			req:     GridVerifyRequest{ImageIDs: []string{}},
			wantOK:  false,
			wantCor: 0,
			wantTot: 2,
		},
		{
			name:    "含空图片ID",
			req:     GridVerifyRequest{ImageIDs: []string{"a:cat1", ""}},
			wantOK:  false,
			wantCor: 0,
			wantTot: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VerifyGrid(chal, tt.req)
			if result.OK != tt.wantOK {
				t.Errorf("OK = %v, want %v (reason: %s)", result.OK, tt.wantOK, result.Reason)
			}
			if result.Correct != tt.wantCor {
				t.Errorf("Correct = %d, want %d", result.Correct, tt.wantCor)
			}
			if result.Total != tt.wantTot {
				t.Errorf("Total = %d, want %d", result.Total, tt.wantTot)
			}
		})
	}
}

func TestVerifyGridNilChallenge(t *testing.T) {
	result := VerifyGrid(nil, GridVerifyRequest{ImageIDs: []string{"a:cat1"}})
	if result.OK {
		t.Error("nil challenge should return not OK")
	}

	result = VerifyGrid(&ChallengeInternal{Type: ChallengeClick}, GridVerifyRequest{})
	if result.OK {
		t.Error("type mismatch should return not OK")
	}
}

func TestVerifyClick(t *testing.T) {
	chal := &ChallengeInternal{
		Type: ChallengeClick,
		Click: &ClickChallengeInternal{
			Regions: []Region{
				{Tag: "cat", X: 10, Y: 10, Width: 50, Height: 50},
				{Tag: "cat", X: 100, Y: 20, Width: 40, Height: 40},
			},
		},
	}

	tests := []struct {
		name    string
		req     ClickVerifyRequest
		wantOK  bool
		wantCor int
		wantTot int
	}{
		{
			name: "全部命中",
			req: ClickVerifyRequest{Points: []ClickPoint{
				{X: 30, Y: 30}, {X: 120, Y: 40},
			}},
			wantOK:  true,
			wantCor: 2,
			wantTot: 2,
		},
		{
			name: "部分命中",
			req: ClickVerifyRequest{Points: []ClickPoint{
				{X: 30, Y: 30},
			}},
			wantOK:  false,
			wantCor: 1,
			wantTot: 2,
		},
		{
			name: "点在区域边界上",
			req: ClickVerifyRequest{Points: []ClickPoint{
				{X: 60, Y: 60}, {X: 100, Y: 20},
			}},
			wantOK:  true,
			wantCor: 2,
			wantTot: 2,
		},
		{
			name: "点不在任何区域",
			req: ClickVerifyRequest{Points: []ClickPoint{
				{X: 5, Y: 5},
			}},
			wantOK:  false,
			wantCor: 0,
			wantTot: 2,
		},
		{
			name: "空点击",
			req: ClickVerifyRequest{Points: []ClickPoint{}},
			wantOK:  false,
			wantCor: 0,
			wantTot: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VerifyClick(chal, tt.req)
			if result.OK != tt.wantOK {
				t.Errorf("OK = %v, want %v (reason: %s)", result.OK, tt.wantOK, result.Reason)
			}
			if result.Correct != tt.wantCor {
				t.Errorf("Correct = %d, want %d", result.Correct, tt.wantCor)
			}
			if result.Total != tt.wantTot {
				t.Errorf("Total = %d, want %d", result.Total, tt.wantTot)
			}
		})
	}
}

func TestVerifyClickNilChallenge(t *testing.T) {
	result := VerifyClick(nil, ClickVerifyRequest{Points: []ClickPoint{{X: 30, Y: 30}}})
	if result.OK {
		t.Error("nil challenge should return not OK")
	}
}

func TestPointInRegion(t *testing.T) {
	r := Region{X: 10, Y: 10, Width: 50, Height: 30}

	tests := []struct {
		name string
		p    ClickPoint
		want bool
	}{
		{"inside", ClickPoint{X: 20, Y: 20}, true},
		{"top-left corner", ClickPoint{X: 10, Y: 10}, true},
		{"bottom-right corner", ClickPoint{X: 60, Y: 40}, true},
		{"outside left", ClickPoint{X: 5, Y: 20}, false},
		{"outside right", ClickPoint{X: 61, Y: 20}, false},
		{"outside top", ClickPoint{X: 20, Y: 5}, false},
		{"outside bottom", ClickPoint{X: 20, Y: 41}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pointInRegion(tt.p, r); got != tt.want {
				t.Errorf("pointInRegion(%v, %v) = %v, want %v", tt.p, r, got, tt.want)
			}
		})
	}
}
