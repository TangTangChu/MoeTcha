package core

import (
	"fmt"
	"sync"
	"time"
)

type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]ChallengeSession
	clock    func() time.Time
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]ChallengeSession),
		clock:    time.Now,
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

func (s *MemorySessionStore) getLocked(id string) (ChallengeSession, bool) {
	ss, ok := s.sessions[id]
	if !ok {
		return ChallengeSession{}, false
	}
	if !ss.ExpiresAt.IsZero() && s.clock().After(ss.ExpiresAt) {
		delete(s.sessions, id)
		return ChallengeSession{}, false
	}
	if ss.MaxAttempts > 0 && ss.Attempts >= ss.MaxAttempts {
		delete(s.sessions, id)
		return ChallengeSession{}, false
	}
	return ss, true
}
