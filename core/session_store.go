package core

import "time"

type ChallengeSession struct {
	ID          string
	Challenge   *ChallengeInternal
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Attempts    int
	MaxAttempts int
	IP          string
	UserAgent   string
}

type SessionStore interface {
	Save(session ChallengeSession) error
	Get(id string) (ChallengeSession, bool)
	Delete(id string) error
	IncrementAttempt(id string) (ChallengeSession, bool)
}
