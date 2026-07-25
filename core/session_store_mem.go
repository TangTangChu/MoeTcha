package core

import (
	"fmt"
	"sync"
	"time"
)

type attemptResult struct {
	At time.Time
	OK bool
}

type MemorySessionStore struct {
	mu        sync.Mutex
	sessions  map[string]ChallengeSession
	clock     func() time.Time
	byIP      map[string]map[string]struct{}
	attempts  map[string][]time.Time
	results   map[string][]attemptResult
	spent     map[string]time.Time
	ipBuckets map[string]*rateBucket
	uaBuckets map[string]*rateBucket
	blockedIP map[string]time.Time
	blockedUA map[string]time.Time
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions:  make(map[string]ChallengeSession),
		clock:     time.Now,
		byIP:      make(map[string]map[string]struct{}),
		attempts:  make(map[string][]time.Time),
		results:   make(map[string][]attemptResult),
		spent:     make(map[string]time.Time),
		ipBuckets: make(map[string]*rateBucket),
		uaBuckets: make(map[string]*rateBucket),
		blockedIP: make(map[string]time.Time),
		blockedUA: make(map[string]time.Time),
	}
}

func (s *MemorySessionStore) Save(session ChallengeSession) error {
	if session.ID == "" {
		return fmt.Errorf("session ID 为空")
	}
	if session.Challenge == nil {
		return fmt.Errorf("session challenge 为空")
	}
	if session.ExpiresAt.IsZero() {
		return fmt.Errorf("session expiresAt 为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	if session.IP != "" {
		set, ok := s.byIP[session.IP]
		if !ok {
			set = make(map[string]struct{})
			s.byIP[session.IP] = set
		}
		set[session.ID] = struct{}{}
	}
	return nil
}

func (s *MemorySessionStore) Get(id string) (ChallengeSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}

func (s *MemorySessionStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(id)
	delete(s.sessions, id)
	return nil
}

func (s *MemorySessionStore) IncrementAttempt(id string) (ChallengeSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.getLocked(id)
	if !ok {
		return ChallengeSession{}, false
	}
	ss.Attempts++
	s.sessions[id] = ss
	return ss, true
}

func (s *MemorySessionStore) ValidateActiveCount(ip string, max int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if max <= 0 {
		return nil
	}
	set, ok := s.byIP[ip]
	if !ok {
		return nil
	}
	if len(set) >= max {
		return fmt.Errorf("IP 活跃 session 过多")
	}
	return nil
}

func (s *MemorySessionStore) AllowAttempt(ip string, max int, window time.Duration) bool {
	if ip == "" {
		return true
	}
	if max <= 0 || window <= 0 {
		return true
	}
	cutoff := s.clock().Add(-window)
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.attempts[ip]
	kept := list[:0]
	for _, t := range list {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= max {
		s.attempts[ip] = kept
		return false
	}
	kept = append(kept, s.clock())
	s.attempts[ip] = kept
	return true
}

func (s *MemorySessionStore) AllowFailRatio(ip string, ratio float64, window time.Duration) bool {
	if ip == "" {
		return true
	}
	if ratio <= 0 || window <= 0 {
		return true
	}
	cutoff := s.clock().Add(-window)
	s.mu.Lock()
	defer s.mu.Unlock()
	results := s.results[ip]
	if len(results) == 0 {
		return true
	}
	attempts := s.attempts[ip]
	filteredAttempts := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			filteredAttempts = append(filteredAttempts, t)
		}
	}
	if len(filteredAttempts) == 0 {
		s.attempts[ip] = filteredAttempts
		return true
	}
	maxChecks := 0
	fail := 0
	for i := len(results) - 1; i >= 0; i-- {
		if results[i].At.Before(cutoff) {
			break
		}
		maxChecks++
		if !results[i].OK {
			fail++
		}
		if maxChecks >= len(filteredAttempts) {
			break
		}
	}
	if maxChecks == 0 {
		s.attempts[ip] = filteredAttempts
		return true
	}
	if float64(fail)/float64(maxChecks) >= ratio {
		s.attempts[ip] = filteredAttempts
		return false
	}
	s.attempts[ip] = filteredAttempts
	return true
}

func (s *MemorySessionStore) RecordOutcome(ip string, ok bool) {
	if ip == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.results[ip]
	list = append(list, attemptResult{At: s.clock(), OK: ok})
	if len(list) > 200 {
		list = list[len(list)-200:]
	}
	s.results[ip] = list
}

func (s *MemorySessionStore) getLocked(id string) (ChallengeSession, bool) {
	ss, ok := s.sessions[id]
	if !ok {
		return ChallengeSession{}, false
	}
	if !ss.ExpiresAt.IsZero() && s.clock().After(ss.ExpiresAt) {
		s.deleteLocked(id)
		delete(s.sessions, id)
		return ChallengeSession{}, false
	}
	if ss.MaxAttempts > 0 && ss.Attempts >= ss.MaxAttempts {
		s.deleteLocked(id)
		delete(s.sessions, id)
		return ChallengeSession{}, false
	}
	return ss, true
}

func (s *MemorySessionStore) deleteLocked(id string) {
	ss, ok := s.sessions[id]
	if !ok {
		return
	}
	if ss.IP == "" {
		return
	}
	if set, ok := s.byIP[ss.IP]; ok {
		delete(set, id)
		if len(set) == 0 {
			delete(s.byIP, ss.IP)
		}
	}
}

func (s *MemorySessionStore) SignToken(sessionID string, ctx VerifyContext, policy TokenPolicy) (string, error) {
	if policy.SigningKey == "" {
		return "", fmt.Errorf("TOKEN_SIGNING_KEY 为空")
	}
	claims := buildTokenClaims(sessionID, ctx, policy)
	return signToken(claims, policy.SigningKey), nil
}

func (s *MemorySessionStore) VerifyToken(sessionID string, ctx VerifyContext, policy TokenPolicy) error {
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
	if policy.SingleUse {
		if !s.markTokenUsed(claims.ID, policy.TTL) {
			return fmt.Errorf("token 已使用")
		}
	}
	if err := verifyTokenBinding(claims, ctx, policy); err != nil {
		return err
	}
	return nil
}

func (s *MemorySessionStore) Allow(ctx VerifyContext, policy RateLimitPolicy) bool {
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

func (s *MemorySessionStore) markTokenUsed(id string, ttl time.Duration) bool {
	if id == "" {
		return false
	}
	now := s.clock()
	if ttl <= 0 {
		return false
	}
	exp := now.Add(ttl)
	if exp.Before(now) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.spent[id]; ok {
		if t.After(now) {
			return false
		}
	}
	s.spent[id] = exp
	return true
}
