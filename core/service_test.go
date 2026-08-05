package core

import (
	"testing"
	"time"
)

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
