package core

import (
	"errors"
	"os"
	"strings"
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

// stubLookup 让下列测试完全不依赖进程环境，保持 hermetic。
func stubLookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// 改造前 mustInt/mustDuration 等把解析错误静默吞掉并回落默认值，
// 运维改一个配置项要重启一次才能发现下一个错。现在必须一次性全报出来。
func TestLoadCollectsAllErrors(t *testing.T) {
	cfg, _, err := Load(LoadOptions{Lookup: stubLookup(map[string]string{
		"CAPTCHA_TTL":            "2min",
		"CAPTCHA_MAX_ATTEMPTS":   "three",
		"CAPTCHA_DIFFICULTY":     "hardcore",
		"CAPTCHA_IP_ENABLED":     "yesplease",
		"CAPTCHA_MAX_FAIL_RATIO": "半",
	})})
	if err == nil {
		t.Fatal("期望报错，实际通过")
	}

	var cfgErrs ConfigErrors
	if !errors.As(err, &cfgErrs) {
		t.Fatalf("错误类型应为 ConfigErrors，实际=%T", err)
	}
	if len(cfgErrs) != 5 {
		t.Fatalf("应报出 5 项错误，实际 %d 项：%v", len(cfgErrs), err)
	}

	msg := err.Error()
	for _, key := range []string{
		"CAPTCHA_TTL", "CAPTCHA_MAX_ATTEMPTS", "CAPTCHA_DIFFICULTY",
		"CAPTCHA_IP_ENABLED", "CAPTCHA_MAX_FAIL_RATIO",
	} {
		if !strings.Contains(msg, key) {
			t.Errorf("错误信息未提及 %s：%s", key, msg)
		}
	}

	// 出错项回落默认值，保证 Config 始终完整可展示。
	if cfg.Service.TTL != 2*time.Minute {
		t.Errorf("出错项应回落默认值，TTL = %v", cfg.Service.TTL)
	}
	if cfg.Service.MaxAttempts != 3 {
		t.Errorf("出错项应回落默认值，MaxAttempts = %d", cfg.Service.MaxAttempts)
	}
	if cfg.Service.Difficulty != DiffEasy {
		t.Errorf("出错项应回落默认值，Difficulty = %q", cfg.Service.Difficulty)
	}
}

// 宽松模式供 config show 使用：配置有错也要能完整渲染出来。
func TestLoadLenientReportsPerItemErrors(t *testing.T) {
	cfg, resolved, err := Load(LoadOptions{
		Lenient: true,
		Lookup:  stubLookup(map[string]string{"CAPTCHA_TTL": "2min"}),
	})
	if err != nil {
		t.Fatalf("宽松模式不应返回错误：%v", err)
	}
	if len(resolved) != len(Specs()) {
		t.Fatalf("应返回全部 %d 项，实际 %d 项", len(Specs()), len(resolved))
	}
	if cfg.Service.TTL != 2*time.Minute {
		t.Errorf("TTL 应回落默认值，实际=%v", cfg.Service.TTL)
	}

	var bad int
	for _, rv := range resolved {
		if rv.Err != nil {
			bad++
			if rv.Spec.Key != "CAPTCHA_TTL" {
				t.Errorf("意外的出错项：%s", rv.Spec.Key)
			}
		}
	}
	if bad != 1 {
		t.Errorf("应有 1 项标记为错误，实际 %d 项", bad)
	}
}

// 优先级：命令行 > 真实环境变量 > .env 文件 > 默认值。
// 真实环境变量压过 .env 是安全要求——镜像里烤进的 .env 不得覆盖 docker run -e 传入的生产密钥。
func TestLoadPrecedence(t *testing.T) {
	cfg, resolved, err := Load(LoadOptions{
		Flags:  map[string]string{"CAPTCHA_MAX_ATTEMPTS": "9"},
		DotEnv: map[string]string{"HTTP_PORT": "7777", "CAPTCHA_TTL": "9m", "CAPTCHA_MAX_ATTEMPTS": "5"},
		Lookup: stubLookup(map[string]string{"HTTP_PORT": "8888", "CAPTCHA_MAX_ATTEMPTS": "7"}),
	})
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}

	if cfg.HTTPPort != "8888" {
		t.Errorf("环境变量应压过 .env，HTTPPort = %q，期望 8888", cfg.HTTPPort)
	}
	if cfg.Service.TTL != 9*time.Minute {
		t.Errorf("仅 .env 提供时应生效，TTL = %v，期望 9m", cfg.Service.TTL)
	}
	if cfg.Service.MaxAttempts != 9 {
		t.Errorf("命令行应压过一切，MaxAttempts = %d，期望 9", cfg.Service.MaxAttempts)
	}
	if cfg.Storage.Backend != "memory" {
		t.Errorf("未提供时应用默认值，Backend = %q", cfg.Storage.Backend)
	}

	sources := map[string]EnvSource{}
	for _, rv := range resolved {
		sources[rv.Spec.Key] = rv.Source
	}
	for key, want := range map[string]EnvSource{
		"HTTP_PORT":            SourceEnv,
		"CAPTCHA_TTL":          SourceDotEnv,
		"CAPTCHA_MAX_ATTEMPTS": SourceFlag,
		"STORAGE_BACKEND":      SourceDefault,
	} {
		if sources[key] != want {
			t.Errorf("%s 来源 = %v，期望 %v", key, sources[key], want)
		}
	}
}

// 保留改造前 getEnv 的语义：空值等同于未设置，回落默认值。
func TestLoadEmptyValueFallsBackToDefault(t *testing.T) {
	cfg, _, err := Load(LoadOptions{Lookup: stubLookup(map[string]string{
		"HTTP_PORT":       "",
		"STORAGE_BACKEND": "   ",
	})})
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}
	if cfg.HTTPPort != "8080" {
		t.Errorf("空值应回落默认值，HTTPPort = %q", cfg.HTTPPort)
	}
	if cfg.Storage.Backend != "memory" {
		t.Errorf("全空白应回落默认值，Backend = %q", cfg.Storage.Backend)
	}
}

func TestLoadMasksSecretsInResolvedValues(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	_, resolved, err := Load(LoadOptions{Lookup: stubLookup(map[string]string{
		"CAPTCHA_TOKEN_SIGNING_KEY": key,
	})})
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}

	for _, rv := range resolved {
		if rv.Spec.Key != "CAPTCHA_TOKEN_SIGNING_KEY" {
			continue
		}
		if strings.Contains(rv.Value, key) {
			t.Errorf("Value 泄露了密钥：%q", rv.Value)
		}
		if rv.Raw != key {
			t.Errorf("Raw 应保留原值供 --show-secrets 使用，实际=%q", rv.Raw)
		}
		return
	}
	t.Fatal("未找到 CAPTCHA_TOKEN_SIGNING_KEY")
}

// LOG_LEVEL 改造前是空配置：写在 .env.example 里但没有任何代码读取。
func TestLogLevelIsWired(t *testing.T) {
	cfg, _, err := Load(LoadOptions{Lookup: stubLookup(nil)})
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel 默认值 = %q，期望 info", cfg.LogLevel)
	}

	cfg, _, err = Load(LoadOptions{Lookup: stubLookup(map[string]string{"LOG_LEVEL": "DEBUG"})})
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LOG_LEVEL 应转小写，实际=%q", cfg.LogLevel)
	}

	if _, _, err = Load(LoadOptions{Lookup: stubLookup(map[string]string{"LOG_LEVEL": "verbose"})}); err == nil {
		t.Error("非法 LOG_LEVEL 应当报错而非静默回落")
	}
}
