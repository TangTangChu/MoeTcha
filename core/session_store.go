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
	// LastAttemptAt 是上一次校验尝试的时间，用于 MinVerifyInterval 节流重试。
	// 零值表示尚未尝试过，校验时回退到 CreatedAt。
	LastAttemptAt time.Time
}

type SessionStore interface {
	Save(session ChallengeSession) error
	Get(id string) (ChallengeSession, bool)
	Delete(id string) error
	IncrementAttempt(id string) (ChallengeSession, bool)
}
