package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"moetcha/core"
)

func TestParseSetFlags(t *testing.T) {
	got, err := parseSetFlags([]string{"HTTP_PORT=9000", "CAPTCHA_TTL=5m"})
	if err != nil {
		t.Fatalf("parseSetFlags 失败：%v", err)
	}
	if got["HTTP_PORT"] != "9000" || got["CAPTCHA_TTL"] != "5m" {
		t.Errorf("解析结果不符：%v", got)
	}
}

func TestParseSetFlagsRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		item string
	}{
		{"缺少等号", "HTTP_PORT"},
		{"键名为空", "=9000"},
		// 打错键名却静默生效，比直接报错更难排查。
		{"未知键名", "HTTP_PORTT=9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseSetFlags([]string{tt.item}); err == nil {
				t.Errorf("%q 应当报错", tt.item)
			}
		})
	}
}

func TestParseSetFlagsAllowsEmptyValue(t *testing.T) {
	got, err := parseSetFlags([]string{"API_TOKENS="})
	if err != nil {
		t.Fatalf("空值应被接受：%v", err)
	}
	if v, ok := got["API_TOKENS"]; !ok || v != "" {
		t.Errorf("API_TOKENS = %q（存在=%v），期望空字符串", v, ok)
	}
}

func TestPresetDevProducesValidConfig(t *testing.T) {
	cfg := loadPreset(t, "dev")
	if cfg.Storage.Backend != "memory" {
		t.Errorf("dev 预设应使用内存存储，实际=%q", cfg.Storage.Backend)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("dev 预设应使用 debug 日志，实际=%q", cfg.LogLevel)
	}
	if cfg.Service.Secure.Token.Enabled {
		t.Error("dev 预设不应启用 Token")
	}
}

// prod 预设必须开箱即通过校验——启用了 Token 却没生成密钥会直接启动失败。
func TestPresetProdProducesValidConfig(t *testing.T) {
	cfg := loadPreset(t, "prod")
	if cfg.Storage.Backend != "sqlite" {
		t.Errorf("prod 预设应使用 sqlite，实际=%q", cfg.Storage.Backend)
	}
	if !cfg.Service.Secure.Token.Enabled {
		t.Error("prod 预设应启用 Token")
	}
	if len(cfg.Service.Secure.Token.SigningKey) != 64 {
		t.Errorf("签名密钥应为 64 位十六进制，实际长度=%d", len(cfg.Service.Secure.Token.SigningKey))
	}
	if len(cfg.Service.APIAuth.Tokens) != 1 {
		t.Fatalf("prod 预设应生成 1 个 API Token，实际=%d", len(cfg.Service.APIAuth.Tokens))
	}
	if !cfg.Service.Secure.RateLimit.Enabled {
		t.Error("prod 预设应启用限流")
	}
}

// 走完「预设 → 模板 → 解析 → 加载 → 校验」全链路，确保生成的文件真能用。
func loadPreset(t *testing.T, preset string) core.Config {
	t.Helper()

	overrides, err := presetOverrides(preset)
	if err != nil {
		t.Fatalf("生成 %s 预设失败：%v", preset, err)
	}
	content := core.RenderDotEnvTemplate(overrides)

	entries, err := core.ParseDotEnv(strings.NewReader(content), preset)
	if err != nil {
		t.Fatalf("生成的 .env 无法解析：%v", err)
	}
	cfg, _, err := core.Load(core.LoadOptions{
		DotEnv: core.DotEnvMap(entries),
		Lookup: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("加载 %s 预设失败：%v", preset, err)
	}
	if err := core.ValidateConfig(cfg); err != nil {
		t.Fatalf("%s 预设未通过校验：%v", preset, err)
	}
	return cfg
}

// 每次生成的密钥必须不同，否则等于所有部署共用一个密钥。
func TestPresetSecretsAreRandom(t *testing.T) {
	a, err := presetOverrides("prod")
	if err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	b, err := presetOverrides("prod")
	if err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	if a["CAPTCHA_TOKEN_SIGNING_KEY"] == b["CAPTCHA_TOKEN_SIGNING_KEY"] {
		t.Error("两次生成的签名密钥相同")
	}
	if a["API_TOKENS"] == b["API_TOKENS"] {
		t.Error("两次生成的 API Token 相同")
	}
	if a["CAPTCHA_TOKEN_SIGNING_KEY"] == a["API_TOKENS"] {
		t.Error("签名密钥与 API Token 不应相同")
	}
}

func TestRandomHexLength(t *testing.T) {
	got, err := randomHex(32)
	if err != nil {
		t.Fatalf("randomHex 失败：%v", err)
	}
	if len(got) != 64 {
		t.Errorf("32 字节应输出 64 个十六进制字符，实际=%d", len(got))
	}
}

func TestDisplayValueRespectsSecrets(t *testing.T) {
	spec, ok := core.SpecByKey("CAPTCHA_TOKEN_SIGNING_KEY")
	if !ok {
		t.Fatal("注册表缺少 CAPTCHA_TOKEN_SIGNING_KEY")
	}
	const raw = "0123456789abcdef"
	rv := core.ResolvedValue{Spec: spec, Raw: raw, Value: spec.Display(raw)}

	if got := displayValue(rv, false); strings.Contains(got, raw) {
		t.Errorf("默认应脱敏，实际=%q", got)
	}
	if got := displayValue(rv, true); got != raw {
		t.Errorf("--show-secrets 应显示原值，实际=%q", got)
	}
}

func TestDefaultOf(t *testing.T) {
	if got := defaultOf("HTTP_PORT"); got != "8080" {
		t.Errorf("HTTP_PORT 默认值 = %q，期望 8080", got)
	}
	if got := defaultOf("NOT_A_REAL_KEY"); got != "" {
		t.Errorf("未知键应返回空字符串，实际=%q", got)
	}
}

func TestRunUnknownCommandExitsWithUsageError(t *testing.T) {
	if code := Run([]string{"moetcha", "nonsense"}); code != 2 {
		t.Errorf("未知命令应返回 2，实际=%d", code)
	}
}

func TestRunVersionAndHelp(t *testing.T) {
	for _, arg := range []string{"version", "help"} {
		if code := Run([]string{"moetcha", arg}); code != 0 {
			t.Errorf("%s 应返回 0，实际=%d", arg, code)
		}
	}
}

func TestRunConfigWithoutSubcommand(t *testing.T) {
	if code := Run([]string{"moetcha", "config"}); code != 2 {
		t.Errorf("config 缺子命令应返回 2，实际=%d", code)
	}
}

func TestSplitFlagsAndPositional(t *testing.T) {
	vf := map[string]bool{"env-file": true, "set": true}
	tests := []struct {
		name      string
		args      []string
		wantFlags []string
		wantPos   []string
	}{
		{"仅位置参数", []string{"FOO"}, nil, []string{"FOO"}},
		{"标志在前", []string{"--env-file", "x", "FOO"}, []string{"--env-file", "x"}, []string{"FOO"}},
		{"位置在前", []string{"FOO", "--env-file", "x"}, []string{"--env-file", "x"}, []string{"FOO"}},
		{"等号形式", []string{"--env-file=x", "FOO"}, []string{"--env-file=x"}, []string{"FOO"}},
		{"多位置夹杂布尔标志", []string{"A", "--show-secrets", "B"}, []string{"--show-secrets"}, []string{"A", "B"}},
		{"终止符后皆位置", []string{"--", "-not-a-flag"}, nil, []string{"-not-a-flag"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, p := splitFlagsAndPositional(tt.args, vf)
			if !eqSlice(f, tt.wantFlags) {
				t.Errorf("flags = %v, want %v", f, tt.wantFlags)
			}
			if !eqSlice(p, tt.wantPos) {
				t.Errorf("positional = %v, want %v", p, tt.wantPos)
			}
		})
	}
}

func eqSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSetEnvLineUpdatesExisting(t *testing.T) {
	content := []byte("# comment\nFOO=1\nBAR=2\n")
	got, existed := setEnvLine(content, "FOO", "9")
	if !existed {
		t.Error("FOO 应已存在")
	}
	gotStr := string(got)
	if !strings.Contains(gotStr, "FOO=9\n") || strings.Contains(gotStr, "FOO=1") {
		t.Errorf("更新结果不符：%q", gotStr)
	}
	if !strings.Contains(gotStr, "BAR=2") {
		t.Errorf("其它行不应受影响：%q", gotStr)
	}
}

func TestSetEnvLineAppendsNew(t *testing.T) {
	content := []byte("FOO=1\n")
	got, existed := setEnvLine(content, "NEW", "x")
	if existed {
		t.Error("NEW 不应已存在")
	}
	if !strings.HasSuffix(string(got), "NEW=x\n") {
		t.Errorf("应追加 NEW=x，实际：%q", string(got))
	}
}

// 精确匹配 KEY=：不能把 FOO 的 set 误改到 FOO_BAR。
func TestSetEnvLineDoesNotMatchPrefix(t *testing.T) {
	content := []byte("FOO=1\nFOO_BAR=2\n")
	got, existed := setEnvLine(content, "FOO", "9")
	if !existed {
		t.Error("FOO 应已存在")
	}
	if !strings.Contains(string(got), "FOO_BAR=2") {
		t.Errorf("FOO_BAR 不应被误改：%q", string(got))
	}
}

// 端到端：init -> set（位置参数在前）-> 文件落地；未知键拒绝。
func TestRunConfigSetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	if code := Run([]string{"moetcha", "config", "init", "--preset", "dev", "--output", envPath}); code != 0 {
		t.Fatalf("config init 失败，exit=%d", code)
	}
	if code := Run([]string{"moetcha", "config", "set", "CAPTCHA_TTL=30s", "--env-file", envPath}); code != 0 {
		t.Fatalf("config set 失败，exit=%d", code)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "CAPTCHA_TTL=30s\n") {
		t.Errorf("文件未更新：未找到 CAPTCHA_TTL=30s\n%s", string(data))
	}
	if code := Run([]string{"moetcha", "config", "set", "NOPE=1", "--env-file", envPath}); code != 2 {
		t.Errorf("未知键应返回 2，实际=%d", code)
	}
}
