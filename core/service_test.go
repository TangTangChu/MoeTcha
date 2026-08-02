package core

import (
	"testing"
	"time"
)

// TestVerifyMinIntervalThrottlesRetries 回归：MinVerifyInterval 的配置语义是
// 「两次校验之间的最小间隔」。旧实现只比 CreatedAt，首次通过后重试不受限；
// 修复后首次以 CreatedAt 为基准、重试以上一次尝试时间(LastAttemptAt)为基准。
//
// Verify 不触碰文件系统/渲染，故可注入 nil 的 Engine/AssetStore/Renderer。
func TestVerifyMinIntervalThrottlesRetries(t *testing.T) {
	store := NewMemorySessionStore()
	svc := &Service{
		SessionStore: store,
		MaxAttempts:  99,
		Secure: SecurePolicy{
			MinVerifyInterval: 100 * time.Millisecond,
			DeleteOnFailed:    false, // 答错不删会话，以便测重试
		},
	}

	chal := &ChallengeInternal{
		Type: ChallengeGrid,
		Tag:  "T",
		Grid: &GridChallengeInternal{
			Images:          []GridItemInternal{{ImageID: "a", Path: ""}},
			CorrectImageIDs: []string{"a"},
		},
	}
	ctx := VerifyContext{IP: "1.2.3.4", UserAgent: "test"}
	badReq := &GridVerifyRequest{ImageIDs: []string{"nonexistent"}}

	saveSession := func(id string) {
		now := time.Now()
		if err := store.Save(ChallengeSession{
			ID:          id,
			Challenge:   chal,
			CreatedAt:   now,
			ExpiresAt:   now.Add(time.Hour),
			MaxAttempts: 99,
		}); err != nil {
			t.Fatalf("Save(%s): %v", id, err)
		}
	}

	// 1) 创建后立即校验 -> TOO_FAST（首次：基准 CreatedAt）
	saveSession("sessA")
	if _, err := svc.Verify("sessA", badReq, nil, ctx); err == nil {
		t.Fatal("首次立即校验：期望 TOO_FAST，得到 nil error")
	} else if ve, ok := err.(*VerifyError); !ok || ve.Code != CodeTooFast {
		t.Fatalf("首次立即校验：期望 TOO_FAST，得到 err=%v", err)
	}

	// 2) 等过间隔后首次校验 -> 放行进入判定（err==nil，答错 not solved）
	saveSession("sessB")
	time.Sleep(150 * time.Millisecond)
	res, err := svc.Verify("sessB", badReq, nil, ctx)
	if err != nil {
		t.Fatalf("间隔后首次校验：期望放行，得到 err=%v", err)
	}
	if res.Solved {
		t.Fatalf("答错却 solved=true: %+v", res)
	}

	// 3) 立即重试 -> TOO_FAST（重试节流：基准上一次尝试时间 LastAttemptAt）
	if _, err := svc.Verify("sessB", badReq, nil, ctx); err == nil {
		t.Fatal("立即重试：期望 TOO_FAST，得到 nil error")
	} else if ve, ok := err.(*VerifyError); !ok || ve.Code != CodeTooFast {
		t.Fatalf("立即重试：期望 TOO_FAST，得到 err=%v", err)
	}
}

// TestVerifyMinIntervalZeroDoesNotGet 回归：MinVerifyInterval=0（默认）时
// 不应触发额外的 Get 调用路径异常，立即校验直接放行。
func TestVerifyMinIntervalZeroAllowsImmediate(t *testing.T) {
	store := NewMemorySessionStore()
	svc := &Service{
		SessionStore: store,
		MaxAttempts:  99,
		Secure:       SecurePolicy{}, // MinVerifyInterval=0
	}
	chal := &ChallengeInternal{
		Type: ChallengeGrid,
		Tag:  "T",
		Grid: &GridChallengeInternal{
			Images:          []GridItemInternal{{ImageID: "a", Path: ""}},
			CorrectImageIDs: []string{"a"},
		},
	}
	now := time.Now()
	if err := store.Save(ChallengeSession{
		ID: "sessZ", Challenge: chal, CreatedAt: now, ExpiresAt: now.Add(time.Hour), MaxAttempts: 99,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	res, err := svc.Verify("sessZ", &GridVerifyRequest{ImageIDs: []string{"a"}}, nil, VerifyContext{IP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("立即校验：期望放行，得到 err=%v", err)
	}
	if !res.Solved {
		t.Fatalf("期望 solved，得到 code=%s", res.Code)
	}
}
