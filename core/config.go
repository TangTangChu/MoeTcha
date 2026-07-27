package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort   string
	Service    ServiceConfig
	Storage    StorageConfig
	SQLitePath string
}

type StorageConfig struct {
	Backend string
}

type ServiceConfig struct {
	TTL         time.Duration
	MaxAttempts int
	Difficulty  Difficulty
	IPPolicy    IPPolicy
	Secure      SecurePolicy
}

func LoadConfig() (Config, error) {
	port := getEnv("HTTP_PORT", "8080")

	diff := Difficulty(strings.ToLower(getEnv("CAPTCHA_DIFFICULTY", "easy")))
	if diff != DiffEasy && diff != DiffMedium && diff != DiffHard {
		return Config{}, fmt.Errorf("CAPTCHA_DIFFICULTY 必须为 easy / medium / hard，当前=%s", diff)
	}

	service := ServiceConfig{
		TTL:         mustDuration("CAPTCHA_TTL", 2*time.Minute),
		MaxAttempts: mustInt("CAPTCHA_MAX_ATTEMPTS", 3),
		Difficulty:  diff,
		IPPolicy: IPPolicy{
			Enabled:      mustBool("CAPTCHA_IP_ENABLED", false),
			RequireMatch: mustBool("CAPTCHA_IP_REQUIRE_MATCH", true),
			MaxActive:    mustInt("CAPTCHA_IP_MAX_ACTIVE", 0),
		},
		Secure: SecurePolicy{
			RequireUserAgent:     mustBool("CAPTCHA_REQUIRE_UA", false),
			RequireSameUserAgent: mustBool("CAPTCHA_REQUIRE_SAME_UA", true),
			DeleteOnFailed:       mustBool("CAPTCHA_DELETE_ON_FAILED", false),
			MaxAttemptsPerIP:     mustInt("CAPTCHA_MAX_ATTEMPTS_IP", 0),
			MaxAttemptsWindow:    mustDuration("CAPTCHA_MAX_ATTEMPTS_IP_WINDOW", 0),
			MinVerifyInterval:    mustDuration("CAPTCHA_MIN_VERIFY_INTERVAL", 0),
			MaxFailRatio:         mustFloat("CAPTCHA_MAX_FAIL_RATIO", 0),
			FailRatioWindow:      mustDuration("CAPTCHA_FAIL_RATIO_WINDOW", 0),
			Token: TokenPolicy{
				Enabled:        mustBool("CAPTCHA_TOKEN_ENABLED", false),
				TTL:            mustDuration("CAPTCHA_TOKEN_TTL", 90*time.Second),
				SingleUse:      mustBool("CAPTCHA_TOKEN_SINGLE_USE", true),
				BindIP:         mustBool("CAPTCHA_TOKEN_BIND_IP", true),
				BindUserAgent:  mustBool("CAPTCHA_TOKEN_BIND_UA", true),
				BindSession:    mustBool("CAPTCHA_TOKEN_BIND_SESSION", true),
				BindIPPrefix:   mustInt("CAPTCHA_TOKEN_BIND_IP_PREFIX", 24),
				SigningKey:     getEnv("CAPTCHA_TOKEN_SIGNING_KEY", ""),
				SigningKeyNext: getEnv("CAPTCHA_TOKEN_SIGNING_KEY_NEXT", ""),
				RotationGrace:  mustDuration("CAPTCHA_TOKEN_ROTATION_GRACE", 0),
			},
			RateLimit: RateLimitPolicy{
				Enabled:    mustBool("CAPTCHA_RATE_LIMIT_ENABLED", false),
				PerIPQPS:   mustInt("CAPTCHA_RATE_LIMIT_IP_QPS", 0),
				PerIPBurst: mustInt("CAPTCHA_RATE_LIMIT_IP_BURST", 0),
				PerUAQPS:   mustInt("CAPTCHA_RATE_LIMIT_UA_QPS", 0),
				PerUABurst: mustInt("CAPTCHA_RATE_LIMIT_UA_BURST", 0),
				BlockTTL:   mustDuration("CAPTCHA_RATE_LIMIT_BLOCK_TTL", 0),
				SoftReject: mustBool("CAPTCHA_RATE_LIMIT_SOFT_REJECT", false),
			},
		},
	}

	storage := StorageConfig{
		Backend: getEnv("STORAGE_BACKEND", "memory"),
	}
	sqlitePath := getEnv("SQLITE_PATH", "./data/moetcha.db")

	return Config{HTTPPort: port, Service: service, Storage: storage, SQLitePath: sqlitePath}, nil
}

func getEnv(key, def string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	return val
}

func mustBool(key string, def bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return def
	}
	return parsed
}

func mustInt(key string, def int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return parsed
}

func mustFloat(key string, def float64) float64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return def
	}
	return parsed
}

func mustDuration(key string, def time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	parsed, err := time.ParseDuration(val)
	if err != nil {
		return def
	}
	return parsed
}

func ValidateConfig(cfg Config) error {
	if cfg.HTTPPort == "" {
		return fmt.Errorf("HTTP_PORT 为空")
	}
	if cfg.Storage.Backend != "memory" && cfg.Storage.Backend != "sqlite" {
		return fmt.Errorf("STORAGE_BACKEND 必须为 memory 或 sqlite")
	}
	if cfg.Storage.Backend == "sqlite" && cfg.SQLitePath == "" {
		return fmt.Errorf("SQLITE_PATH 为空")
	}
	if cfg.Service.IPPolicy.Enabled && cfg.Service.IPPolicy.RequireMatch && cfg.Service.IPPolicy.MaxActive < 0 {
		return fmt.Errorf("CAPTCHA_IP_MAX_ACTIVE 不合法")
	}
	if cfg.Service.Secure.MaxFailRatio < 0 || cfg.Service.Secure.MaxFailRatio > 1 {
		return fmt.Errorf("CAPTCHA_MAX_FAIL_RATIO 必须在 0~1")
	}
	if cfg.Service.Secure.Token.Enabled {
		if cfg.Service.Secure.Token.SigningKey == "" {
			return fmt.Errorf("CAPTCHA_TOKEN_SIGNING_KEY 为空")
		}
	}
	return nil
}
