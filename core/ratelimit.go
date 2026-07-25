package core

import "time"

type rateBucket struct {
	Tokens  float64
	Updated time.Time
}

func allowBucket(buckets map[string]*rateBucket, key string, now time.Time, qps int, burst int) bool {
	if qps <= 0 {
		return true
	}
	if burst <= 0 {
		burst = qps
	}
	b, ok := buckets[key]
	if !ok {
		b = &rateBucket{Tokens: float64(burst), Updated: now}
		buckets[key] = b
	}
	elapsed := now.Sub(b.Updated).Seconds()
	b.Tokens += elapsed * float64(qps)
	if b.Tokens > float64(burst) {
		b.Tokens = float64(burst)
	}
	b.Updated = now
	if b.Tokens < 1 {
		return false
	}
	b.Tokens -= 1
	return true
}

func blocked(blocks map[string]time.Time, key string, now time.Time) bool {
	if exp, ok := blocks[key]; ok {
		if exp.After(now) {
			return true
		}
		delete(blocks, key)
	}
	return false
}

func applyBlock(blocks map[string]time.Time, key string, now time.Time, ttl time.Duration) {
	if ttl <= 0 || key == "" {
		return
	}
	blocks[key] = now.Add(ttl)
}
