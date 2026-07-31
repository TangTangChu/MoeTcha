package core

import (
	"fmt"
	"strings"
	"time"
)

type Config struct {
	HTTPPort   string
	LogLevel   string
	Service    ServiceConfig
	Storage    StorageConfig
	SQLitePath string
	CORS       CORSConfig
}

type StorageConfig struct {
	Backend string
}

// CORSConfig 控制跨域响应头与预检处理，供浏览器端调用方使用。
type CORSConfig struct {
	Enabled        bool
	AllowedOrigins []string
}

type ServiceConfig struct {
	TTL         time.Duration
	MaxAttempts int
	Difficulty  Difficulty
	IPPolicy    IPPolicy
	Secure      SecurePolicy

	// APIAuth 控制内部接口（如 /grid/generate）的 Bearer Token 鉴权。
	// Tokens 为空时这些接口保持开放（仅适合内网/本地开发），非空时强制校验。
	APIAuth APIAuthConfig

	// GridGenerateConcurrency 限制 /grid/generate 同时渲染的请求数，0 表示按 CPU 核数。
	GridGenerateConcurrency int

	// MaxSourceImagePixels 限制单张源图解码后的像素数，超过则拒绝，防止内存放大。0 表示使用默认值。
	MaxSourceImagePixels int
}

type APIAuthConfig struct {
	Tokens []string
}

// LoadOptions 控制配置加载行为。
type LoadOptions struct {
	// DotEnv 为 .env 文件解析结果，优先级低于真实环境变量。
	DotEnv map[string]string
	// Flags 为命令行覆盖（--set K=V），优先级最高。
	Flags map[string]string
	// Lenient 为真时即使存在解析错误也返回 nil error，
	// 供 `config show` 在配置有错的情况下仍完整渲染。
	Lenient bool
	// Lookup 默认为 os.LookupEnv，测试可替换以保持 hermetic。
	Lookup func(string) (string, bool)
}

// Load 依据配置注册表解析全部配置项。
//
// 与逐项解析不同，这里会把所有解析错误收集齐再一次性返回，
// 避免运维改一个错一个地反复重启。出错项回落默认值后继续解析，
// 因此返回的 Config 总是完整可用的。
func Load(opts LoadOptions) (Config, []ResolvedValue, error) {
	var cfg Config
	for _, s := range configSpecs {
		s.setDefault(&cfg)
	}

	r := &Resolver{DotEnv: opts.DotEnv, Flags: opts.Flags, Lookup: opts.Lookup}
	resolved := make([]ResolvedValue, 0, len(configSpecs))
	var errs ConfigErrors

	for _, s := range configSpecs {
		raw, src, found := r.lookup(s.Key)
		if !found {
			effective := s.read(&cfg)
			resolved = append(resolved, ResolvedValue{
				Spec: s, Value: s.Display(effective), Raw: effective, Source: SourceDefault,
			})
			continue
		}

		var applyErr error
		if err := s.apply(&cfg, raw); err != nil {
			// s.Display 保证密钥的坏值不会随错误信息进入日志。
			ce := &ConfigError{Key: s.Key, Value: s.Display(raw), Source: src, Msg: err.Error()}
			errs = append(errs, ce)
			applyErr = ce
		}
		effective := s.read(&cfg)
		resolved = append(resolved, ResolvedValue{
			Spec: s, Value: s.Display(effective), Raw: effective, Source: src, Err: applyErr,
		})
	}

	if len(errs) > 0 && !opts.Lenient {
		return cfg, resolved, errs
	}
	return cfg, resolved, nil
}

// LoadConfig 从进程环境加载配置。
//
// 刻意不读取 .env 文件：Go 测试以包目录为工作目录，若在此自动加载，
// core 包的测试会隐式依赖 core/.env 是否存在而失去 hermetic 性。
// .env 的加载由 CLI 层显式完成。
func LoadConfig() (Config, error) {
	cfg, _, err := Load(LoadOptions{})
	return cfg, err
}

func parseTokenList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	for i, t := range cfg.Service.APIAuth.Tokens {
		if len(t) < 8 {
			return fmt.Errorf("API_TOKENS 第 %d 个 token 长度不足 8（建议使用高强度随机串）", i+1)
		}
	}
	return nil
}
