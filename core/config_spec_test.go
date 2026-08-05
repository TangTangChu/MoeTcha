package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDotEnvExampleMatchesRegistry(t *testing.T) {
	path := filepath.Join("..", ".env.example")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("无法打开 %s：%v", path, err)
	}
	defer f.Close()

	entries, err := ParseDotEnv(f, path)
	if err != nil {
		t.Fatalf("解析 %s 失败：%v", path, err)
	}
	got := DotEnvMap(entries)

	specs := Specs()
	for _, s := range specs {
		value, ok := got[s.Key]
		if !ok {
			t.Errorf("%s 缺少 %s（请运行 moetcha config template --output .env.example 重新生成）", path, s.Key)
			continue
		}
		if value != s.Default {
			t.Errorf("%s 中 %s=%q，注册表默认值为 %q", path, s.Key, value, s.Default)
		}
	}

	for key := range got {
		if _, ok := SpecByKey(key); !ok {
			t.Errorf("%s 含有注册表中不存在的 %s", path, key)
		}
	}

	if len(got) != len(specs) {
		t.Errorf("%s 有 %d 项，注册表有 %d 项", path, len(got), len(specs))
	}
}

// 注册表里的默认值字符串必须能被自己的解析器接受，
// 否则 bindSpec 会在包初始化时 panic。这里显式覆盖以便定位。
func TestSpecDefaultsAreParseable(t *testing.T) {
	var cfg Config
	for _, s := range Specs() {
		s.setDefault(&cfg)
		if err := s.apply(&cfg, s.Default); err != nil {
			t.Errorf("%s 的默认值 %q 无法解析：%v", s.Key, s.Default, err)
		}
	}
}

func TestSpecMetadataIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Specs() {
		if seen[s.Key] {
			t.Errorf("%s 在注册表中重复出现", s.Key)
		}
		seen[s.Key] = true

		if s.Section == "" {
			t.Errorf("%s 缺少 Section", s.Key)
		}
		if s.Desc == "" {
			t.Errorf("%s 缺少 Desc", s.Key)
		}
		if s.Kind == KindEnum && len(s.Enum) == 0 {
			t.Errorf("%s 是枚举但未声明可选值", s.Key)
		}
	}
}

// 三个密钥项必须标记为 Secret，否则会明文出现在 config show 与错误信息里。
func TestSecretSpecsAreMarked(t *testing.T) {
	want := []string{"CAPTCHA_TOKEN_SIGNING_KEY", "CAPTCHA_TOKEN_SIGNING_KEY_NEXT", "API_TOKENS"}
	for _, key := range want {
		s, ok := SpecByKey(key)
		if !ok {
			t.Fatalf("注册表缺少 %s", key)
		}
		if !s.Secret {
			t.Errorf("%s 应标记为 Secret", key)
		}
	}
}

func TestSpecDisplayMasksSecrets(t *testing.T) {
	secret, _ := SpecByKey("CAPTCHA_TOKEN_SIGNING_KEY")
	const raw = "abcdef0123456789"
	got := secret.Display(raw)
	if strings.Contains(got, raw) {
		t.Errorf("Display 泄露了密钥：%q", got)
	}
	if !strings.Contains(got, "16") {
		t.Errorf("Display 应提示长度，实际=%q", got)
	}

	plain, _ := SpecByKey("HTTP_PORT")
	if plain.Display("8080") != "8080" {
		t.Errorf("非密钥项不应脱敏，实际=%q", plain.Display("8080"))
	}
}

func TestRenderDotEnvTemplateAppliesOverrides(t *testing.T) {
	content := RenderDotEnvTemplate(map[string]string{
		"HTTP_PORT":       "9999",
		"STORAGE_BACKEND": "sqlite",
	})

	entries, err := ParseDotEnv(strings.NewReader(content), "template")
	if err != nil {
		t.Fatalf("生成的模板无法被解析：%v", err)
	}
	got := DotEnvMap(entries)

	if got["HTTP_PORT"] != "9999" {
		t.Errorf("HTTP_PORT = %q，期望 9999", got["HTTP_PORT"])
	}
	if got["STORAGE_BACKEND"] != "sqlite" {
		t.Errorf("STORAGE_BACKEND = %q，期望 sqlite", got["STORAGE_BACKEND"])
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("未覆盖项应保持默认值，LOG_LEVEL = %q", got["LOG_LEVEL"])
	}
	if len(got) != len(Specs()) {
		t.Errorf("模板含 %d 项，注册表有 %d 项", len(got), len(Specs()))
	}
}

// 模板必须能被自己解析并加载成合法配置，形成闭环。
func TestRenderedTemplateLoadsCleanly(t *testing.T) {
	entries, err := ParseDotEnv(strings.NewReader(RenderDotEnvTemplate(nil)), "template")
	if err != nil {
		t.Fatalf("解析模板失败：%v", err)
	}
	cfg, _, err := Load(LoadOptions{
		DotEnv: DotEnvMap(entries),
		Lookup: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("默认模板应当能干净加载：%v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("默认模板应当通过校验：%v", err)
	}
}
