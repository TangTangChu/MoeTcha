package core

import (
	"testing"
	"time"
)

func TestParseTrustedNetworks(t *testing.T) {
	tests := []struct {
		name    string
		raw     []string
		wantErr bool
		// 抽样断言：ip -> 是否可信
		checks map[string]bool
	}{
		{
			name: "private 关键字展开",
			raw:  []string{"private"},
			checks: map[string]bool{
				"10.0.0.1":    true,
				"192.168.1.5": true,
				"172.16.0.2":  true,
				"127.0.0.1":   true,
				"::1":         true,
				"8.8.8.8":     false,
				"203.0.113.1": false,
				"":            false,
			},
		},
		{
			name: "显式 CIDR",
			raw:  []string{"10.1.0.0/16", "203.0.113.0/24"},
			checks: map[string]bool{
				"10.1.2.3":    true,
				"10.2.0.1":    false,
				"203.0.113.5": true,
				"8.8.8.8":     false,
			},
		},
		{
			name: "裸 IP 自动补 /32",
			raw:  []string{"9.9.9.9"},
			checks: map[string]bool{
				"9.9.9.9": true,
				"9.9.9.8": false,
			},
		},
		{
			name:    "非法条目报错",
			raw:     []string{"10.0.0.0/8", "not-a-cidr"},
			wantErr: true,
			checks: map[string]bool{
				"10.0.0.1": true, // 成功条目仍生效
			},
		},
		{
			name: "空列表放行全部不可信",
			raw:  nil,
			checks: map[string]bool{
				"10.0.0.1":  false,
				"127.0.0.1": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nets, err := ParseTrustedNetworks(tt.raw)
			if tt.wantErr && err == nil {
				t.Fatal("期望解析错误，得到 nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("不期望错误，得到 %v", err)
			}
			for ip, want := range tt.checks {
				if got := isTrustedIP(ip, nets); got != want {
					t.Errorf("isTrustedIP(%q) = %v, want %v", ip, got, want)
				}
			}
		})
	}
}

// TestVerifyTrustedSkipsUAMatch 回归：可信网络的请求即使 UA 与签发时不一致也放行。
func TestVerifyTrustedSkipsUAMatch(t *testing.T) {
	store := NewMemorySessionStore()
	nets, _ := ParseTrustedNetworks([]string{"10.0.0.0/8"})
	svc := &Service{
		SessionStore: store,
		MaxAttempts:  99,
		Secure: SecurePolicy{
			RequireSameUserAgent: true, // 默认即开启
		},
		TrustedNets: nets,
	}
	chal := &ChallengeInternal{
		Type: ChallengeGrid,
		Grid: &GridChallengeInternal{
			Images:          []GridItemInternal{{ImageID: "a", Path: ""}},
			CorrectImageIDs: []string{"a"},
		},
	}
	now := time.Now()
	// session 签发时 UA=signer；verify 时用完全不同的 UA，IP 落在可信网段。
	if err := store.Save(ChallengeSession{
		ID: "sessT", Challenge: chal, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		MaxAttempts: 99, IP: "10.0.0.1", UserAgent: "signer",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	res, err := svc.Verify("sessT", &GridVerifyRequest{ImageIDs: []string{"a"}}, nil,
		VerifyContext{IP: "10.0.0.1", UserAgent: "totally-different"})
	if err != nil {
		t.Fatalf("可信网络 UA 不一致：期望放行，得到 err=%v", err)
	}
	if !res.Solved {
		t.Fatalf("期望 solved，得到 code=%s", res.Code)
	}
}

// TestVerifyUntrustedEnforcesUAMatch 回归：非可信网络仍受 UA 一致性约束。
func TestVerifyUntrustedEnforcesUAMatch(t *testing.T) {
	store := NewMemorySessionStore()
	svc := &Service{
		SessionStore: store,
		MaxAttempts:  99,
		Secure:       SecurePolicy{RequireSameUserAgent: true},
		// TrustedNets 留空
	}
	chal := &ChallengeInternal{
		Type: ChallengeGrid,
		Grid: &GridChallengeInternal{
			Images:          []GridItemInternal{{ImageID: "a", Path: ""}},
			CorrectImageIDs: []string{"a"},
		},
	}
	now := time.Now()
	if err := store.Save(ChallengeSession{
		ID: "sessU", Challenge: chal, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		MaxAttempts: 99, IP: "8.8.8.8", UserAgent: "signer",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_, err := svc.Verify("sessU", &GridVerifyRequest{ImageIDs: []string{"a"}}, nil,
		VerifyContext{IP: "8.8.8.8", UserAgent: "different"})
	if err == nil {
		t.Fatal("非可信网络 UA 不一致：期望 UA_MISMATCH，得到 nil")
	}
	ve, ok := err.(*VerifyError)
	if !ok || ve.Code != CodeUAMismatch {
		t.Fatalf("期望 UA_MISMATCH，得到 %v", err)
	}
}
