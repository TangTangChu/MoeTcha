package core

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear relevant env vars
	for _, k := range []string{
		"HTTP_PORT", "STORAGE_BACKEND", "SQLITE_PATH",
		"CAPTCHA_TTL", "CAPTCHA_MAX_ATTEMPTS",
		"CAPTCHA_IP_ENABLED", "CAPTCHA_IP_REQUIRE_MATCH", "CAPTCHA_IP_MAX_ACTIVE",
		"CAPTCHA_REQUIRE_UA", "CAPTCHA_REQUIRE_SAME_UA",
		"CAPTCHA_DELETE_ON_FAILED", "CAPTCHA_MAX_ATTEMPTS_IP", "CAPTCHA_MAX_ATTEMPTS_IP_WINDOW",
		"CAPTCHA_MIN_VERIFY_INTERVAL", "CAPTCHA_MAX_FAIL_RATIO", "CAPTCHA_FAIL_RATIO_WINDOW",
		"CAPTCHA_TOKEN_ENABLED", "CAPTCHA_TOKEN_TTL", "CAPTCHA_TOKEN_SINGLE_USE",
		"CAPTCHA_TOKEN_BIND_IP", "CAPTCHA_TOKEN_BIND_UA", "CAPTCHA_TOKEN_BIND_SESSION",
		"CAPTCHA_TOKEN_BIND_IP_PREFIX", "CAPTCHA_TOKEN_SIGNING_KEY", "CAPTCHA_TOKEN_SIGNING_KEY_NEXT",
		"CAPTCHA_TOKEN_ROTATION_GRACE",
		"CAPTCHA_RATE_LIMIT_ENABLED", "CAPTCHA_RATE_LIMIT_IP_QPS", "CAPTCHA_RATE_LIMIT_IP_BURST",
		"CAPTCHA_RATE_LIMIT_UA_QPS", "CAPTCHA_RATE_LIMIT_UA_BURST", "CAPTCHA_RATE_LIMIT_BLOCK_TTL",
		"CAPTCHA_RATE_LIMIT_SOFT_REJECT",
	} {
		os.Unsetenv(k)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.HTTPPort != "8080" {
		t.Errorf("HTTPPort = %q, want \"8080\"", cfg.HTTPPort)
	}
	if cfg.Storage.Backend != "memory" {
		t.Errorf("Storage.Backend = %q, want \"memory\"", cfg.Storage.Backend)
	}
	if cfg.SQLitePath != "./data/moetcha.db" {
		t.Errorf("SQLitePath = %q, want \"./data/moetcha.db\"", cfg.SQLitePath)
	}
	if cfg.Service.TTL != 2*time.Minute {
		t.Errorf("TTL = %v, want 2m", cfg.Service.TTL)
	}
	if cfg.Service.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.Service.MaxAttempts)
	}
	if cfg.Service.IPPolicy.Enabled != false {
		t.Error("IPPolicy.Enabled should default to false")
	}
	if cfg.Service.Secure.Token.Enabled != false {
		t.Error("Token.Enabled should default to false")
	}
	if cfg.Service.Secure.RateLimit.Enabled != false {
		t.Error("RateLimit.Enabled should default to false")
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("STORAGE_BACKEND", "sqlite")
	t.Setenv("SQLITE_PATH", ":memory:")
	t.Setenv("CAPTCHA_TTL", "5m")
	t.Setenv("CAPTCHA_MAX_ATTEMPTS", "5")
	t.Setenv("CAPTCHA_IP_ENABLED", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.HTTPPort != "9090" {
		t.Errorf("HTTPPort = %q, want \"9090\"", cfg.HTTPPort)
	}
	if cfg.Storage.Backend != "sqlite" {
		t.Errorf("Storage.Backend = %q, want \"sqlite\"", cfg.Storage.Backend)
	}
	if cfg.SQLitePath != ":memory:" {
		t.Errorf("SQLitePath = %q, want \":memory:\"", cfg.SQLitePath)
	}
	if cfg.Service.TTL != 5*time.Minute {
		t.Errorf("TTL = %v, want 5m", cfg.Service.TTL)
	}
	if cfg.Service.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", cfg.Service.MaxAttempts)
	}
	if cfg.Service.IPPolicy.Enabled != true {
		t.Error("IPPolicy.Enabled should be true")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid defaults",
			cfg: Config{
				HTTPPort: "8080",
				Storage:  StorageConfig{Backend: "memory"},
			},
			wantErr: false,
		},
		{
			name: "valid sqlite",
			cfg: Config{
				HTTPPort:   "8080",
				Storage:    StorageConfig{Backend: "sqlite"},
				SQLitePath: ":memory:",
			},
			wantErr: false,
		},
		{
			name: "empty port",
			cfg: Config{
				Storage: StorageConfig{Backend: "memory"},
			},
			wantErr: true,
		},
		{
			name: "invalid storage backend",
			cfg: Config{
				HTTPPort: "8080",
				Storage:  StorageConfig{Backend: "redis"},
			},
			wantErr: true,
		},
		{
			name: "sqlite without path",
			cfg: Config{
				HTTPPort: "8080",
				Storage:  StorageConfig{Backend: "sqlite"},
			},
			wantErr: true,
		},
		{
			name: "max fail ratio out of range",
			cfg: Config{
				HTTPPort: "8080",
				Storage:  StorageConfig{Backend: "memory"},
				Service: ServiceConfig{
					Secure: SecurePolicy{MaxFailRatio: 1.5},
				},
			},
			wantErr: true,
		},
		{
			name: "token enabled without signing key",
			cfg: Config{
				HTTPPort: "8080",
				Storage:  StorageConfig{Backend: "memory"},
				Service: ServiceConfig{
					Secure: SecurePolicy{
						Token: TokenPolicy{Enabled: true},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
