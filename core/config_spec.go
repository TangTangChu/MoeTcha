package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ValueKind 描述配置项的取值类型，用于渲染帮助与格式提示。
type ValueKind int

const (
	KindString ValueKind = iota
	KindBool
	KindInt
	KindFloat
	KindDuration
	KindEnum
	KindList
)

func (k ValueKind) String() string {
	switch k {
	case KindBool:
		return "布尔"
	case KindInt:
		return "整数"
	case KindFloat:
		return "小数"
	case KindDuration:
		return "时长"
	case KindEnum:
		return "枚举"
	case KindList:
		return "列表"
	default:
		return "字符串"
	}
}

// Spec 描述一个配置项。注册表（见 configSpecs）是配置的唯一事实来源：
// LoadConfig 由它驱动，`config show` 由它渲染，.env.example 由它生成。
// 三者共用一份定义，从根本上杜绝文档与代码漂移。
type Spec struct {
	Key     string
	Section string
	Kind    ValueKind
	Default string   // 默认值的字符串形式，与 .env.example 逐字一致
	Secret  bool     // 为真时值在任何输出中都必须脱敏
	Desc    string   // 中文描述，同时用作 .env.example 的注释
	Enum    []string // 可选值，用于文档与交互向导

	setDefault func(*Config)
	apply      func(*Config, string) error
	read       func(*Config) string
}

// Display 返回可安全打印的值：密钥项一律脱敏，空值显式标注。
func (s Spec) Display(v string) string {
	if s.Secret {
		return maskSecret(v)
	}
	if v == "" {
		return `""`
	}
	return v
}

func maskSecret(v string) string {
	if v == "" {
		return `""`
	}
	return fmt.Sprintf("****（长度 %d）", len(v))
}

// bindSpec 把「解析函数 + 字段选择器」组合成类型安全的 Spec。
// base.Default、parse、format、sel 由同一个 T 串起来，编译期即可排除
// 把 int 绑到 *time.Duration 之类的错配，全程不使用反射或 any。
func bindSpec[T any](base Spec, parse func(string) (T, error), format func(T) string, sel func(*Config) *T) Spec {
	def, err := parse(base.Default)
	if err != nil {
		// 注册表是编译期常量，默认值写错属于程序员错误，应立即暴露。
		panic(fmt.Sprintf("配置注册表 %s 的默认值 %q 非法：%v", base.Key, base.Default, err))
	}
	base.setDefault = func(c *Config) { *sel(c) = def }
	base.apply = func(c *Config, raw string) error {
		v, err := parse(raw)
		if err != nil {
			// 回落默认值后继续，保证 Config 始终完整——
			// 这正是 `config show` 能在配置有错时仍完整渲染的前提。
			*sel(c) = def
			return err
		}
		*sel(c) = v
		return nil
	}
	base.read = func(c *Config) string { return format(*sel(c)) }
	return base
}

func specString(base Spec, sel func(*Config) *string) Spec {
	base.Kind = KindString
	return bindSpec(base, func(s string) (string, error) { return s, nil }, func(s string) string { return s }, sel)
}

func specBool(base Spec, sel func(*Config) *bool) Spec {
	base.Kind = KindBool
	return bindSpec(base, parseBoolValue, strconv.FormatBool, sel)
}

func specInt(base Spec, sel func(*Config) *int) Spec {
	base.Kind = KindInt
	return bindSpec(base, parseIntValue, strconv.Itoa, sel)
}

func specInt64(base Spec, sel func(*Config) *int64) Spec {
	base.Kind = KindInt
	return bindSpec(base, parseInt64Value, func(v int64) string { return strconv.FormatInt(v, 10) }, sel)
}

func specFloat(base Spec, sel func(*Config) *float64) Spec {
	base.Kind = KindFloat
	return bindSpec(base, parseFloatValue, formatFloat, sel)
}

func specDuration(base Spec, sel func(*Config) *time.Duration) Spec {
	base.Kind = KindDuration
	return bindSpec(base, parseDurationValue, formatDuration, sel)
}

func specEnum(base Spec, sel func(*Config) *string) Spec {
	base.Kind = KindEnum
	allowed := base.Enum
	parse := func(s string) (string, error) {
		v := strings.ToLower(s)
		for _, a := range allowed {
			if v == a {
				return v, nil
			}
		}
		return "", fmt.Errorf("必须为 %s", strings.Join(allowed, " / "))
	}
	return bindSpec(base, parse, func(s string) string { return s }, sel)
}

func specDifficulty(base Spec, sel func(*Config) *Difficulty) Spec {
	base.Kind = KindEnum
	allowed := base.Enum
	parse := func(s string) (Difficulty, error) {
		v := Difficulty(strings.ToLower(s))
		for _, a := range allowed {
			if string(v) == a {
				return v, nil
			}
		}
		return "", fmt.Errorf("必须为 %s", strings.Join(allowed, " / "))
	}
	return bindSpec(base, parse, func(d Difficulty) string { return string(d) }, sel)
}

func specList(base Spec, sel func(*Config) *[]string) Spec {
	base.Kind = KindList
	parse := func(s string) ([]string, error) { return parseTokenList(s), nil }
	format := func(v []string) string { return strings.Join(v, ",") }
	return bindSpec(base, parse, format, sel)
}

func parseBoolValue(s string) (bool, error) {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return false, fmt.Errorf("必须为布尔值（true / false / 1 / 0）")
	}
	return v, nil
}

func parseIntValue(s string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("必须为整数")
	}
	return v, nil
}

func parseInt64Value(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("必须为整数")
	}
	return v, nil
}

func parseFloatValue(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("必须为小数")
	}
	return v, nil
}

func parseDurationValue(s string) (time.Duration, error) {
	v, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("必须为时长格式（如 90s、2m、1h30m）")
	}
	return v, nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	return d.String()
}

// configSpecs 是全部配置项的唯一事实来源，顺序即 .env.example 的生成顺序。
// 默认值即各配置项的字面量初值，不得随意变动。
var configSpecs = []Spec{
	// ── 服务 ──
	specString(Spec{
		Key: "HTTP_PORT", Section: "服务", Default: "8080",
		Desc: "HTTP 监听端口",
	}, func(c *Config) *string { return &c.HTTPPort }),
	specString(Spec{
		Key: "HTTP_HOST", Section: "服务", Default: "",
		Desc: "HTTP 监听地址，留空表示监听所有网络接口",
	}, func(c *Config) *string { return &c.HTTPHost }),

	// ── 存储 ──
	specString(Spec{
		Key: "STORAGE_BACKEND", Section: "存储", Default: "memory",
		Desc: "存储后端：memory 或 sqlite", Enum: []string{"memory", "sqlite"},
	}, func(c *Config) *string { return &c.Storage.Backend }),
	specString(Spec{
		Key: "SQLITE_PATH", Section: "存储", Default: "./data/moetcha.db",
		Desc: "SQLite 数据库文件路径，STORAGE_BACKEND=sqlite 时生效",
	}, func(c *Config) *string { return &c.SQLitePath }),

	// ── 日志 ──
	specEnum(Spec{
		Key: "LOG_LEVEL", Section: "日志", Default: "info",
		Desc: "日志级别", Enum: []string{"debug", "info", "warn", "error"},
	}, func(c *Config) *string { return &c.LogLevel }),

	// ── CORS 跨域 ──
	specBool(Spec{
		Key: "CORS_ENABLED", Section: "CORS 跨域", Default: "true",
		Desc: "是否启用 CORS 跨域响应头（浏览器端调用必需）",
	}, func(c *Config) *bool { return &c.CORS.Enabled }),
	specList(Spec{
		Key: "CORS_ALLOWED_ORIGINS", Section: "CORS 跨域", Default: "*",
		Desc: "允许的跨域来源列表（逗号分隔），* 表示放行任意来源",
	}, func(c *Config) *[]string { return &c.CORS.AllowedOrigins }),

	// ── CAPTCHA 基础 ──
	specDifficulty(Spec{
		Key: "CAPTCHA_DIFFICULTY", Section: "CAPTCHA 基础", Default: "easy",
		Desc: "验证码难度", Enum: []string{"easy", "medium", "hard"},
	}, func(c *Config) *Difficulty { return &c.Service.Difficulty }),
	specDuration(Spec{
		Key: "CAPTCHA_TTL", Section: "CAPTCHA 基础", Default: "2m",
		Desc: "验证码有效期",
	}, func(c *Config) *time.Duration { return &c.Service.TTL }),
	specInt(Spec{
		Key: "CAPTCHA_MAX_ATTEMPTS", Section: "CAPTCHA 基础", Default: "3",
		Desc: "单个验证码允许的最大尝试次数",
	}, func(c *Config) *int { return &c.Service.MaxAttempts }),

	// ── IP 相关策略 ──
	specBool(Spec{
		Key: "CAPTCHA_IP_ENABLED", Section: "IP 相关策略", Default: "false",
		Desc: "是否启用 IP 策略",
	}, func(c *Config) *bool { return &c.Service.IPPolicy.Enabled }),
	specBool(Spec{
		Key: "CAPTCHA_IP_REQUIRE_MATCH", Section: "IP 相关策略", Default: "true",
		Desc: "校验时是否要求 IP 与签发时一致",
	}, func(c *Config) *bool { return &c.Service.IPPolicy.RequireMatch }),
	specInt(Spec{
		Key: "CAPTCHA_IP_MAX_ACTIVE", Section: "IP 相关策略", Default: "0",
		Desc: "单 IP 同时存活的验证码上限，0 表示不限制",
	}, func(c *Config) *int { return &c.Service.IPPolicy.MaxActive }),

	// ── UA 相关策略 ──
	specBool(Spec{
		Key: "CAPTCHA_REQUIRE_UA", Section: "UA 相关策略", Default: "false",
		Desc: "是否要求请求携带 User-Agent",
	}, func(c *Config) *bool { return &c.Service.Secure.RequireUserAgent }),
	specBool(Spec{
		Key: "CAPTCHA_REQUIRE_SAME_UA", Section: "UA 相关策略", Default: "true",
		Desc: "校验时是否要求 User-Agent 与签发时一致",
	}, func(c *Config) *bool { return &c.Service.Secure.RequireSameUserAgent }),

	// ── 验证强度 ──
	specBool(Spec{
		Key: "CAPTCHA_DELETE_ON_FAILED", Section: "验证强度", Default: "false",
		Desc: "验证失败后是否立即销毁会话",
	}, func(c *Config) *bool { return &c.Service.Secure.DeleteOnFailed }),
	specInt(Spec{
		Key: "CAPTCHA_MAX_ATTEMPTS_IP", Section: "验证强度", Default: "0",
		Desc: "单 IP 在窗口期内的最大尝试次数，0 表示不限制",
	}, func(c *Config) *int { return &c.Service.Secure.MaxAttemptsPerIP }),
	specDuration(Spec{
		Key: "CAPTCHA_MAX_ATTEMPTS_IP_WINDOW", Section: "验证强度", Default: "0",
		Desc: "上述尝试次数的统计窗口，0 表示不限制",
	}, func(c *Config) *time.Duration { return &c.Service.Secure.MaxAttemptsWindow }),
	specDuration(Spec{
		Key: "CAPTCHA_MIN_VERIFY_INTERVAL", Section: "验证强度", Default: "0",
		Desc: "两次校验之间的最小间隔，0 表示不限制",
	}, func(c *Config) *time.Duration { return &c.Service.Secure.MinVerifyInterval }),

	// ── 失败率控制 ──
	specFloat(Spec{
		Key: "CAPTCHA_MAX_FAIL_RATIO", Section: "失败率控制", Default: "0",
		Desc: "允许的最大失败率（0~1），0 表示不限制",
	}, func(c *Config) *float64 { return &c.Service.Secure.MaxFailRatio }),
	specDuration(Spec{
		Key: "CAPTCHA_FAIL_RATIO_WINDOW", Section: "失败率控制", Default: "0",
		Desc: "失败率的统计窗口，0 表示不限制",
	}, func(c *Config) *time.Duration { return &c.Service.Secure.FailRatioWindow }),

	// ── Token 签名与绑定 ──
	specBool(Spec{
		Key: "CAPTCHA_TOKEN_ENABLED", Section: "Token 签名与绑定", Default: "false",
		Desc: "是否签发校验通过后的 Token",
	}, func(c *Config) *bool { return &c.Service.Secure.Token.Enabled }),
	specDuration(Spec{
		Key: "CAPTCHA_TOKEN_TTL", Section: "Token 签名与绑定", Default: "90s",
		Desc: "Token 有效期",
	}, func(c *Config) *time.Duration { return &c.Service.Secure.Token.TTL }),
	specBool(Spec{
		Key: "CAPTCHA_TOKEN_SINGLE_USE", Section: "Token 签名与绑定", Default: "true",
		Desc: "Token 是否一次性使用",
	}, func(c *Config) *bool { return &c.Service.Secure.Token.SingleUse }),
	specBool(Spec{
		Key: "CAPTCHA_TOKEN_BIND_IP", Section: "Token 签名与绑定", Default: "true",
		Desc: "Token 是否绑定 IP",
	}, func(c *Config) *bool { return &c.Service.Secure.Token.BindIP }),
	specBool(Spec{
		Key: "CAPTCHA_TOKEN_BIND_UA", Section: "Token 签名与绑定", Default: "true",
		Desc: "Token 是否绑定 User-Agent",
	}, func(c *Config) *bool { return &c.Service.Secure.Token.BindUserAgent }),
	specBool(Spec{
		Key: "CAPTCHA_TOKEN_BIND_SESSION", Section: "Token 签名与绑定", Default: "true",
		Desc: "Token 是否绑定会话 ID",
	}, func(c *Config) *bool { return &c.Service.Secure.Token.BindSession }),
	specInt(Spec{
		Key: "CAPTCHA_TOKEN_BIND_IP_PREFIX", Section: "Token 签名与绑定", Default: "24",
		Desc: "绑定 IP 时使用的前缀长度",
	}, func(c *Config) *int { return &c.Service.Secure.Token.BindIPPrefix }),
	specString(Spec{
		Key: "CAPTCHA_TOKEN_SIGNING_KEY", Section: "Token 签名与绑定", Default: "", Secret: true,
		Desc: "Token 签名密钥，启用 Token 时必填（可用 moetcha gen-key 生成）",
	}, func(c *Config) *string { return &c.Service.Secure.Token.SigningKey }),
	specString(Spec{
		Key: "CAPTCHA_TOKEN_SIGNING_KEY_NEXT", Section: "Token 签名与绑定", Default: "", Secret: true,
		Desc: "轮换中的下一个签名密钥，留空表示不轮换",
	}, func(c *Config) *string { return &c.Service.Secure.Token.SigningKeyNext }),
	specDuration(Spec{
		Key: "CAPTCHA_TOKEN_ROTATION_GRACE", Section: "Token 签名与绑定", Default: "0",
		Desc: "密钥轮换的宽限期，0 表示不启用",
	}, func(c *Config) *time.Duration { return &c.Service.Secure.Token.RotationGrace }),

	// ── 限流 ──
	specBool(Spec{
		Key: "CAPTCHA_RATE_LIMIT_ENABLED", Section: "限流", Default: "false",
		Desc: "是否启用限流",
	}, func(c *Config) *bool { return &c.Service.Secure.RateLimit.Enabled }),
	specInt(Spec{
		Key: "CAPTCHA_RATE_LIMIT_IP_QPS", Section: "限流", Default: "0",
		Desc: "单 IP 每秒请求数上限，0 表示不限制",
	}, func(c *Config) *int { return &c.Service.Secure.RateLimit.PerIPQPS }),
	specInt(Spec{
		Key: "CAPTCHA_RATE_LIMIT_IP_BURST", Section: "限流", Default: "0",
		Desc: "单 IP 的突发容量，0 表示不限制",
	}, func(c *Config) *int { return &c.Service.Secure.RateLimit.PerIPBurst }),
	specInt(Spec{
		Key: "CAPTCHA_RATE_LIMIT_UA_QPS", Section: "限流", Default: "0",
		Desc: "单 UA 每秒请求数上限，0 表示不限制",
	}, func(c *Config) *int { return &c.Service.Secure.RateLimit.PerUAQPS }),
	specInt(Spec{
		Key: "CAPTCHA_RATE_LIMIT_UA_BURST", Section: "限流", Default: "0",
		Desc: "单 UA 的突发容量，0 表示不限制",
	}, func(c *Config) *int { return &c.Service.Secure.RateLimit.PerUABurst }),
	specDuration(Spec{
		Key: "CAPTCHA_RATE_LIMIT_BLOCK_TTL", Section: "限流", Default: "0",
		Desc: "触发限流后的封禁时长，0 表示不封禁",
	}, func(c *Config) *time.Duration { return &c.Service.Secure.RateLimit.BlockTTL }),
	specBool(Spec{
		Key: "CAPTCHA_RATE_LIMIT_SOFT_REJECT", Section: "限流", Default: "false",
		Desc: "限流时是否软拒绝（返回提示而非直接断开）",
	}, func(c *Config) *bool { return &c.Service.Secure.RateLimit.SoftReject }),

	// ── 客户端 IP 解析 ──
	specEnum(Spec{
		Key: "CLIENT_IP_SOURCE", Section: "客户端 IP 解析", Default: "direct",
		Desc: "客户端 IP 提取来源",
		Enum: []string{"direct", "x-forwarded-for", "x-real-ip"},
	}, func(c *Config) *string { return &c.IPResolve.Source }),
	specInt(Spec{
		Key: "CLIENT_IP_XFF_INDEX", Section: "客户端 IP 解析", Default: "0",
		Desc: "X-Forwarded-For 取第几个元素：0=最左，-1=最右。越界或缺头回落直连 IP",
	}, func(c *Config) *int { return &c.IPResolve.XFFIndex }),

	// ── 可信网络 ──
	specList(Spec{
		Key: "TRUSTED_NETWORKS", Section: "可信网络", Default: "",
		Desc: "可信网络列表（CIDR / 裸 IP / private 关键字）。命中者跳过 IP/UA 一致性、限流、IP 尝试上限与失败率、token 绑定；验证码完整性不受影响",
	}, func(c *Config) *[]string { return &c.Service.TrustedNetworks }),

	// ── 接口鉴权与资源限制 ──
	specList(Spec{
		Key: "API_TOKENS", Section: "接口鉴权与资源限制", Default: "", Secret: true,
		Desc: "逗号分隔的 API Token 列表，为空时 /grid/generate 等内部接口保持开放（仅适合内网/本地开发）；" +
			"配置后客户端需带 Authorization: Bearer <token> 或 X-API-Token: <token>",
	}, func(c *Config) *[]string { return &c.Service.APIAuth.Tokens }),
	specInt(Spec{
		Key: "GRID_GENERATE_CONCURRENCY", Section: "接口鉴权与资源限制", Default: "0",
		Desc: "/grid/generate 同时渲染的并发上限，0 表示按 CPU 核数",
	}, func(c *Config) *int { return &c.Service.GridGenerateConcurrency }),
	specInt(Spec{
		Key: "MAX_SOURCE_IMAGE_PIXELS", Section: "接口鉴权与资源限制", Default: "0",
		Desc: "单张源图解码后的像素数上限，超过则拒绝（防止内存放大）。0 表示默认 16000000",
	}, func(c *Config) *int { return &c.Service.MaxSourceImagePixels }),
	specInt(Spec{
		Key: "GRID_WEBP_METHOD", Section: "接口鉴权与资源限制", Default: "0",
		Desc: "/grid/generate 合成图的 libwebp effort（0=最快，6=最慢/质量最高）。0=自动：小图用 4、中图用 2、大图用 1，避免大网格编码耗时几秒",
	}, func(c *Config) *int { return &c.Service.GridWebPMethod }),

	// ── 图像渲染 ──
	specBool(Spec{
		Key: "RENDER_NOISE_ENABLED", Section: "图像渲染", Default: "false",
		Desc: "是否对验证码图片叠加随机噪声（抗 OCR）。默认关闭，产出干净高质量图；需要干扰时再开启",
	}, func(c *Config) *bool { return &c.Render.NoiseEnabled }),
	specFloat(Spec{
		Key: "RENDER_NOISE_DENSITY", Section: "图像渲染", Default: "0.02",
		Desc: "噪声密度（0~0.2），仅在 RENDER_NOISE_ENABLED=true 时生效。0 等同关闭",
	}, func(c *Config) *float64 { return &c.Render.NoiseDensity }),
	specInt64(Spec{
		Key: "RENDER_NOISE_SEED", Section: "图像渲染", Default: "0",
		Desc: "噪声随机种子，0 表示每张图随机（推荐）。非零值会产生固定噪声图样，仅用于调试/复现",
	}, func(c *Config) *int64 { return &c.Render.NoiseSeed }),
	specInt(Spec{
		Key: "RENDER_QUALITY", Section: "图像渲染", Default: "80",
		Desc: "/challenge 响应图的 WebP 编码质量（1~100）。0 表示默认 80。调高可提升清晰度但增大体积",
	}, func(c *Config) *int { return &c.Render.Quality }),
}

// Specs 返回全部配置项定义，顺序稳定。
func Specs() []Spec { return configSpecs }

// SpecByKey 按环境变量名查找配置项定义。
func SpecByKey(key string) (Spec, bool) {
	for _, s := range configSpecs {
		if s.Key == key {
			return s, true
		}
	}
	return Spec{}, false
}

// RenderDotEnvTemplate 依据注册表生成 .env 模板。
// overrides 用于注入向导答案或预设值，未覆盖的项使用默认值。
func RenderDotEnvTemplate(overrides map[string]string) string {
	var sb strings.Builder
	sb.WriteString("# MoeTcha 配置文件\n")
	sb.WriteString("# 本文件由 `moetcha config template` 生成，请勿手工调整结构。\n")
	sb.WriteString("# 优先级：命令行 > 真实环境变量 > 本文件 > 默认值。\n")

	section := ""
	for _, s := range configSpecs {
		if s.Section != section {
			section = s.Section
			fmt.Fprintf(&sb, "\n# ==== %s ====\n", section)
		}
		value := s.Default
		if v, ok := overrides[s.Key]; ok {
			value = v
		}
		if s.Desc != "" {
			fmt.Fprintf(&sb, "\n# %s\n", s.Desc)
		}
		if len(s.Enum) > 0 {
			fmt.Fprintf(&sb, "# 可选值：%s\n", strings.Join(s.Enum, " / "))
		}
		fmt.Fprintf(&sb, "%s=%s\n", s.Key, value)
	}
	return sb.String()
}
