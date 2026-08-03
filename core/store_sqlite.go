package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id             TEXT PRIMARY KEY,
    challenge_json TEXT    NOT NULL,
    created_at     INTEGER NOT NULL,
    expires_at     INTEGER NOT NULL,
    attempts       INTEGER NOT NULL DEFAULT 0,
    max_attempts   INTEGER NOT NULL DEFAULT 3,
    ip             TEXT    NOT NULL DEFAULT '',
    user_agent     TEXT    NOT NULL DEFAULT '',
    last_attempt_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_ip ON sessions(ip);

CREATE TABLE IF NOT EXISTS assets (
    key        TEXT PRIMARY KEY,
    bytes      BLOB    NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS spent_tokens (
    id         TEXT PRIMARY KEY,
    expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS ip_attempts (
    ip        TEXT    NOT NULL,
    ts        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ip_attempts_ip_ts ON ip_attempts(ip, ts);

CREATE TABLE IF NOT EXISTS ip_results (
    ip TEXT    NOT NULL,
    ts INTEGER NOT NULL,
    ok INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ip_results_ip_ts ON ip_results(ip, ts);
`

type SQLiteSessionStore struct {
	db    *sql.DB
	clock func() time.Time
	mu    sync.Mutex

	// In-memory rate limiting state (high-frequency, not worth persisting)
	ipBuckets map[string]*rateBucket
	uaBuckets map[string]*rateBucket
	blockedIP map[string]time.Time
	blockedUA map[string]time.Time

	done chan struct{}
}

func NewSQLiteSessionStore(dbPath string) (*SQLiteSessionStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("设置 pragma 失败 (%s): %w", p, err)
		}
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("执行 schema 迁移失败: %w", err)
	}
	if err := migrateSessionsLastAttemptAt(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("迁移 sessions.last_attempt_at 失败: %w", err)
	}

	store := &SQLiteSessionStore{
		db:        db,
		clock:     time.Now,
		ipBuckets: make(map[string]*rateBucket),
		uaBuckets: make(map[string]*rateBucket),
		blockedIP: make(map[string]time.Time),
		blockedUA: make(map[string]time.Time),
		done:      make(chan struct{}),
	}

	go store.cleanupLoop()

	return store, nil
}

// migrateSessionsLastAttemptAt 为旧库的 sessions 表补齐 last_attempt_at 列。
// CREATE TABLE IF NOT EXISTS 不会给已存在的表加列，故需要显式迁移。
// 列已存在时（重复迁移）SQLite 返回 "duplicate column name"，视作成功。
func migrateSessionsLastAttemptAt(db *sql.DB) error {
	_, err := db.Exec("ALTER TABLE sessions ADD COLUMN last_attempt_at INTEGER NOT NULL DEFAULT 0")
	if err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		}
		return err
	}
	return nil
}

func (s *SQLiteSessionStore) DB() *sql.DB {
	return s.db
}

func (s *SQLiteSessionStore) Close() error {
	close(s.done)
	return s.db.Close()
}

// --- cleanup ---

func (s *SQLiteSessionStore) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.done:
			return
		}
	}
}

func (s *SQLiteSessionStore) cleanup() {
	now := s.clock().Unix()
	cutoff := now - 3600

	// Must hold mutex? No — these are DELETE-only and SQLite is WAL.
	s.db.Exec("DELETE FROM sessions WHERE expires_at < ? OR (max_attempts > 0 AND attempts >= max_attempts)", now)
	s.db.Exec("DELETE FROM assets WHERE expires_at < ?", now)
	s.db.Exec("DELETE FROM spent_tokens WHERE expires_at < ?", now)
	s.db.Exec("DELETE FROM ip_attempts WHERE ts < ?", cutoff)
	s.db.Exec("DELETE FROM ip_results WHERE ts < ?", cutoff)
}

// --- SessionStore ---

func (s *SQLiteSessionStore) Save(session ChallengeSession) error {
	if session.ID == "" {
		return fmt.Errorf("session ID 为空")
	}
	if session.Challenge == nil {
		return fmt.Errorf("session challenge 为空")
	}
	if session.ExpiresAt.IsZero() {
		return fmt.Errorf("session expiresAt 为空")
	}

	chalJSON, err := json.Marshal(session.Challenge)
	if err != nil {
		return fmt.Errorf("序列化 challenge 失败: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, challenge_json, created_at, expires_at, attempts, max_attempts, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, string(chalJSON),
		session.CreatedAt.Unix(), session.ExpiresAt.Unix(),
		session.Attempts, session.MaxAttempts,
		session.IP, session.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("保存 session 失败: %w", err)
	}
	return nil
}

func (s *SQLiteSessionStore) Get(id string) (ChallengeSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}

func (s *SQLiteSessionStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

func (s *SQLiteSessionStore) IncrementAttempt(id string) (ChallengeSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ss, ok := s.getLocked(id)
	if !ok {
		return ChallengeSession{}, false
	}
	ss.Attempts++
	ss.LastAttemptAt = s.clock()
	_, err := s.db.Exec("UPDATE sessions SET attempts = ?, last_attempt_at = ? WHERE id = ?", ss.Attempts, ss.LastAttemptAt.Unix(), id)
	if err != nil {
		return ChallengeSession{}, false
	}
	return ss, true
}

func (s *SQLiteSessionStore) getLocked(id string) (ChallengeSession, bool) {
	row := s.db.QueryRow(
		"SELECT id, challenge_json, created_at, expires_at, attempts, max_attempts, ip, user_agent, last_attempt_at FROM sessions WHERE id = ?",
		id,
	)

	var ss ChallengeSession
	var chalJSON string
	var createdUnix, expiresUnix, lastAttemptUnix int64
	err := row.Scan(&ss.ID, &chalJSON, &createdUnix, &expiresUnix, &ss.Attempts, &ss.MaxAttempts, &ss.IP, &ss.UserAgent, &lastAttemptUnix)
	if err != nil {
		return ChallengeSession{}, false
	}

	ss.CreatedAt = time.Unix(createdUnix, 0)
	ss.ExpiresAt = time.Unix(expiresUnix, 0)
	ss.LastAttemptAt = time.Unix(lastAttemptUnix, 0)

	// Check expiry and max attempts (same logic as memory store)
	if !ss.ExpiresAt.IsZero() && s.clock().After(ss.ExpiresAt) {
		s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
		return ChallengeSession{}, false
	}
	if ss.MaxAttempts > 0 && ss.Attempts >= ss.MaxAttempts {
		s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
		return ChallengeSession{}, false
	}

	var chal ChallengeInternal
	if err := json.Unmarshal([]byte(chalJSON), &chal); err != nil {
		return ChallengeSession{}, false
	}
	ss.Challenge = &chal
	return ss, true
}

// --- IPSessionTracker ---

func (s *SQLiteSessionStore) ValidateActiveCount(ip string, max int) error {
	if max <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE ip = ?", ip).Scan(&count)
	if err != nil {
		return err
	}
	if count >= max {
		return fmt.Errorf("IP 活跃 session 过多")
	}
	return nil
}

// --- IPAttemptTracker ---

func (s *SQLiteSessionStore) AllowAttempt(ip string, max int, window time.Duration) bool {
	if ip == "" {
		return true
	}
	if max <= 0 || window <= 0 {
		return true
	}

	now := s.clock()
	cutoff := now.Add(-window).Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean old entries
	s.db.Exec("DELETE FROM ip_attempts WHERE ts < ?", cutoff)

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM ip_attempts WHERE ip = ? AND ts >= ?", ip, cutoff).Scan(&count)
	if count >= max {
		return false
	}

	s.db.Exec("INSERT INTO ip_attempts (ip, ts) VALUES (?, ?)", ip, now.Unix())
	return true
}

func (s *SQLiteSessionStore) RecordOutcome(ip string, ok bool) {
	if ip == "" {
		return
	}
	okInt := 0
	if ok {
		okInt = 1
	}
	now := s.clock().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.db.Exec("INSERT INTO ip_results (ip, ts, ok) VALUES (?, ?, ?)", ip, now, okInt)

	// Cap at 200 per IP
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM ip_results WHERE ip = ?", ip).Scan(&count)
	if count > 200 {
		excess := count - 200
		s.db.Exec("DELETE FROM ip_results WHERE ip = ? AND ts IN (SELECT ts FROM ip_results WHERE ip = ? ORDER BY ts ASC LIMIT ?)",
			ip, ip, excess)
	}
}

func (s *SQLiteSessionStore) AllowFailRatio(ip string, ratio float64, window time.Duration) bool {
	if ip == "" {
		return true
	}
	if ratio <= 0 || window <= 0 {
		return true
	}

	now := s.clock()
	cutoff := now.Add(-window).Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM ip_attempts WHERE ip = ? AND ts >= ?", ip, cutoff).Scan(&total)
	if total == 0 {
		return true
	}

	var fail int
	s.db.QueryRow("SELECT COUNT(*) FROM ip_results WHERE ip = ? AND ts >= ? AND ok = 0", ip, cutoff).Scan(&fail)

	if fail == 0 {
		return true
	}
	if float64(fail)/float64(total) >= ratio {
		return false
	}
	return true
}

// --- TokenSigner ---

func (s *SQLiteSessionStore) SignToken(sessionID string, ctx VerifyContext, policy TokenPolicy) (string, error) {
	if policy.SigningKey == "" {
		return "", fmt.Errorf("TOKEN_SIGNING_KEY 为空")
	}
	claims := buildTokenClaims(sessionID, ctx, policy)
	return signToken(claims, policy.SigningKey), nil
}

// --- TokenVerifier ---

func (s *SQLiteSessionStore) VerifyToken(sessionID string, ctx VerifyContext, policy TokenPolicy) error {
	if policy.SigningKey == "" {
		return fmt.Errorf("TOKEN_SIGNING_KEY 为空")
	}
	if ctx.Token == "" {
		return fmt.Errorf("缺少 token")
	}
	claims, err := verifyToken(ctx.Token, policy)
	if err != nil {
		return err
	}
	if policy.BindSession {
		if claims.SessionID != sessionID {
			return fmt.Errorf("token session 不匹配")
		}
	}
	// 先校验绑定（IP/UA），再消费单次 token。否则一个 IP/UA 不匹配的 token
	// 会被烧掉，导致合法持有者（正确 IP/UA）再也无法使用。
	if err := verifyTokenBinding(claims, ctx, policy); err != nil {
		return err
	}
	if policy.SingleUse {
		if !s.markTokenUsed(claims.ID, policy.TTL) {
			return fmt.Errorf("token 已使用")
		}
	}
	return nil
}

func (s *SQLiteSessionStore) markTokenUsed(id string, ttl time.Duration) bool {
	if id == "" {
		return false
	}
	now := s.clock()
	if ttl <= 0 {
		ttl = 2 * time.Minute // 与 buildTokenClaims 的兜底一致，避免 SingleUse 永远失败
	}
	exp := now.Add(ttl)
	if exp.Before(now) {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already used and not expired
	var existingExp int64
	err := s.db.QueryRow("SELECT expires_at FROM spent_tokens WHERE id = ?", id).Scan(&existingExp)
	if err == nil {
		if time.Unix(existingExp, 0).After(now) {
			return false
		}
	}

	_, err = s.db.Exec("INSERT OR REPLACE INTO spent_tokens (id, expires_at) VALUES (?, ?)", id, exp.Unix())
	return err == nil
}

// --- RateLimiter ---

func (s *SQLiteSessionStore) Allow(ctx VerifyContext, policy RateLimitPolicy) bool {
	if !policy.Enabled {
		return true
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.IP != "" {
		if blocked(s.blockedIP, ctx.IP, now) {
			return false
		}
	}
	if ctx.UserAgent != "" {
		if blocked(s.blockedUA, ctx.UserAgent, now) {
			return false
		}
	}
	if ctx.IP != "" && policy.PerIPQPS > 0 {
		if !allowBucket(s.ipBuckets, ctx.IP, now, policy.PerIPQPS, policy.PerIPBurst) {
			applyBlock(s.blockedIP, ctx.IP, now, policy.BlockTTL)
			return false
		}
	}
	if ctx.UserAgent != "" && policy.PerUAQPS > 0 {
		if !allowBucket(s.uaBuckets, ctx.UserAgent, now, policy.PerUAQPS, policy.PerUABurst) {
			applyBlock(s.blockedUA, ctx.UserAgent, now, policy.BlockTTL)
			return false
		}
	}
	return true
}
