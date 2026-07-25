package core

import (
	"testing"
	"time"
)

func newTestSQLiteStore(t *testing.T) *SQLiteSessionStore {
	t.Helper()
	store, err := NewSQLiteSessionStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func makeTestSession(id string, exp time.Time) ChallengeSession {
	return ChallengeSession{
		ID: id,
		Challenge: &ChallengeInternal{
			Type:     ChallengeGrid,
			Question: "测试题目",
			Tag:      "猫",
			Grid: &GridChallengeInternal{
				Images: []GridItemInternal{
					{ImageID: "a:cat1", Path: "/tmp/cat.webp"},
				},
				CorrectImageIDs: []string{"a:cat1"},
			},
		},
		CreatedAt:   time.Now(),
		ExpiresAt:   exp,
		Attempts:    0,
		MaxAttempts: 3,
		IP:          "192.168.1.1",
		UserAgent:   "test-agent/1.0",
	}
}

func TestSQLiteSessionSaveGet(t *testing.T) {
	store := newTestSQLiteStore(t)
	exp := time.Now().Add(5 * time.Minute)
	session := makeTestSession("test-save-1", exp)

	err := store.Save(session)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, ok := store.Get("test-save-1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.ID != session.ID {
		t.Errorf("ID = %q, want %q", got.ID, session.ID)
	}
	if got.IP != session.IP {
		t.Errorf("IP = %q, want %q", got.IP, session.IP)
	}
	if got.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", got.Attempts)
	}
	if got.Challenge == nil {
		t.Fatal("Challenge is nil")
	}
	if got.Challenge.Type != session.Challenge.Type {
		t.Errorf("Challenge.Type = %q, want %q", got.Challenge.Type, session.Challenge.Type)
	}
	if got.Challenge.Question != session.Challenge.Question {
		t.Errorf("Challenge.Question = %q, want %q", got.Challenge.Question, session.Challenge.Question)
	}
}

func TestSQLiteSessionDelete(t *testing.T) {
	store := newTestSQLiteStore(t)
	exp := time.Now().Add(5 * time.Minute)
	session := makeTestSession("test-del-1", exp)

	store.Save(session)
	_, ok := store.Get("test-del-1")
	if !ok {
		t.Fatal("session should exist after save")
	}

	store.Delete("test-del-1")
	_, ok = store.Get("test-del-1")
	if ok {
		t.Error("session should be gone after delete")
	}
}

func TestSQLiteSessionIncrementAttempt(t *testing.T) {
	store := newTestSQLiteStore(t)
	exp := time.Now().Add(5 * time.Minute)
	session := makeTestSession("test-inc-1", exp)

	store.Save(session)

	ss, ok := store.IncrementAttempt("test-inc-1")
	if !ok {
		t.Fatal("IncrementAttempt returned false")
	}
	if ss.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", ss.Attempts)
	}

	ss, ok = store.IncrementAttempt("test-inc-1")
	if !ok {
		t.Fatal("second IncrementAttempt returned false")
	}
	if ss.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", ss.Attempts)
	}
}

func TestSQLiteSessionExpired(t *testing.T) {
	store := newTestSQLiteStore(t)
	// Set clock to expired time
	store.clock = func() time.Time { return time.Now().Add(10 * time.Minute) }

	session := makeTestSession("test-exp-1", time.Now().Add(-5*time.Minute))
	store.Save(session)

	_, ok := store.Get("test-exp-1")
	if ok {
		t.Error("expired session should not be returned")
	}
}

func TestSQLiteSessionMaxAttempts(t *testing.T) {
	store := newTestSQLiteStore(t)
	exp := time.Now().Add(5 * time.Minute)
	session := makeTestSession("test-max-1", exp)
	session.Attempts = 2
	session.MaxAttempts = 2

	store.Save(session)

	_, ok := store.Get("test-max-1")
	if ok {
		t.Error("session at max attempts should not be returned")
	}
}

func TestSQLiteValidateActiveCount(t *testing.T) {
	store := newTestSQLiteStore(t)
	exp := time.Now().Add(5 * time.Minute)

	// Save 2 sessions for same IP
	for i := 0; i < 2; i++ {
		session := makeTestSession("test-ip-"+string(rune('a'+i)), exp)
		session.IP = "10.0.0.1"
		store.Save(session)
	}

	err := store.ValidateActiveCount("10.0.0.1", 3)
	if err != nil {
		t.Errorf("ValidateActiveCount with max=3 should pass: %v", err)
	}

	err = store.ValidateActiveCount("10.0.0.1", 2)
	if err == nil {
		t.Error("ValidateActiveCount with max=2 should fail")
	}
}

func TestSQLiteIPAttemptTracker(t *testing.T) {
	store := newTestSQLiteStore(t)

	// Allow 3 attempts in 60s window
	for i := 0; i < 3; i++ {
		if !store.AllowAttempt("10.0.0.2", 3, 60*time.Second) {
			t.Fatalf("AllowAttempt %d should pass", i+1)
		}
	}

	if store.AllowAttempt("10.0.0.2", 3, 60*time.Second) {
		t.Error("AllowAttempt should fail at max")
	}

	// Record outcomes
	store.RecordOutcome("10.0.0.2", false)
	store.RecordOutcome("10.0.0.2", false)
	store.RecordOutcome("10.0.0.2", true)

	if store.AllowFailRatio("10.0.0.2", 0.5, 60*time.Second) {
		t.Error("AllowFailRatio should fail with 2/3 failures at 0.5 ratio")
	}
}

func TestSQLiteTokenSignVerify(t *testing.T) {
	store := newTestSQLiteStore(t)
	policy := TokenPolicy{
		Enabled:    true,
		TTL:        5 * time.Minute,
		SingleUse:  true,
		BindIP:     true,
		BindSession: true,
		SigningKey: "test-secret-key-32bytes-long!!",
	}

	ctx := VerifyContext{
		IP:        "192.168.1.100",
		UserAgent: "browser/1.0",
	}
	sessionID := "token-test-session"

	token, err := store.SignToken(sessionID, ctx, policy)
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	// Verify with correct context
	verifyCtx := VerifyContext{
		IP:        "192.168.1.100",
		UserAgent: "browser/1.0",
		Token:     token,
	}
	err = store.VerifyToken(sessionID, verifyCtx, policy)
	if err != nil {
		t.Errorf("VerifyToken failed: %v", err)
	}

	// Second use should fail (single use)
	err = store.VerifyToken(sessionID, verifyCtx, policy)
	if err == nil {
		t.Error("VerifyToken should fail on second use (single use)")
	}
}

func TestSQLiteTokenMissingKey(t *testing.T) {
	store := newTestSQLiteStore(t)
	policy := TokenPolicy{Enabled: true}
	ctx := VerifyContext{IP: "1.2.3.4"}

	_, err := store.SignToken("s", ctx, policy)
	if err == nil {
		t.Error("SignToken should fail without signing key")
	}
}

func TestSQLiteRateLimiter(t *testing.T) {
	store := newTestSQLiteStore(t)
	policy := RateLimitPolicy{
		Enabled:    true,
		PerIPQPS:   2,
		PerIPBurst: 2,
	}

	ctx := VerifyContext{IP: "192.168.1.200"}

	// First 2 should pass (burst)
	if !store.Allow(ctx, policy) {
		t.Error("first request should be allowed")
	}
	if !store.Allow(ctx, policy) {
		t.Error("second request should be allowed")
	}
	// Third should fail (exhausted tokens)
	if store.Allow(ctx, policy) {
		t.Error("third request should be rejected")
	}
}

func TestSQLiteAssetStore(t *testing.T) {
	// Create a session store first to get the DB
	sessStore := newTestSQLiteStore(t)
	assetStore := NewSQLiteAssetStore(sessStore.DB())

	asset := Asset{
		Bytes:     []byte("fake-image-bytes"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	key, err := assetStore.Save(asset)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if key == "" {
		t.Fatal("key is empty")
	}

	got, ok := assetStore.Get(key)
	if !ok {
		t.Fatal("Get returned false")
	}
	if string(got.Bytes) != string(asset.Bytes) {
		t.Errorf("Bytes = %q, want %q", got.Bytes, asset.Bytes)
	}

	// Delete
	assetStore.Delete(key)
	_, ok = assetStore.Get(key)
	if ok {
		t.Error("asset should be gone after delete")
	}
}

func TestSQLiteAssetExpired(t *testing.T) {
	sessStore := newTestSQLiteStore(t)
	assetStore := NewSQLiteAssetStore(sessStore.DB())
	assetStore.clock = func() time.Time { return time.Now().Add(10 * time.Minute) }

	asset := Asset{
		Bytes:     []byte("expired-bytes"),
		CreatedAt: time.Now().Add(-10 * time.Minute),
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}

	key, err := assetStore.Save(asset)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	_, ok := assetStore.Get(key)
	if ok {
		t.Error("expired asset should not be returned")
	}
}
