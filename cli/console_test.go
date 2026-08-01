package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"moetcha/core"
)

// consoleTestEnv 构造一个可注入输出的测试用控制台：引擎用内存 packs 构建，
// provider 指向 baseDir（reload 测试用）。
func consoleTestEnv(t *testing.T, baseDir string) (*console, *bytes.Buffer) {
	t.Helper()
	idx, err := core.BuildIndexer(testPacks())
	if err != nil {
		t.Fatalf("BuildIndexer: %v", err)
	}
	engine := core.NewEngine(idx)
	service := &core.Service{Difficulty: core.DiffEasy}
	provider := &core.DirectoryProvider{BaseDir: baseDir, MetaFileName: "meta.json", Strict: true}
	out := &bytes.Buffer{}
	c := newConsole(engine, service, provider, "8080", "memory", "")
	c.out = out
	return c, out
}

// testPacks 返回 11 张「猫」图（够 9 格网格），供构建索引与 reload 前状态使用。
func testPacks() []core.Pack {
	imgs := make([]core.GridImageMeta, 0, 11)
	for i := 1; i <= 11; i++ {
		id := fmt.Sprintf("cat_%02d", i)
		imgs = append(imgs, core.GridImageMeta{
			File:   id + ".webp",
			Tags:   []string{"猫"},
			ID:     id,
			PackID: "animals",
			Path:   "/tmp/" + id + ".webp",
		})
	}
	return []core.Pack{{
		ID:         "animals",
		PackName:   "动物测试包",
		TagDefs:    map[string]core.TagDef{"猫": {Name: "猫"}},
		Grid:       &core.GridConfig{Size: 9, CorrectMin: 2, CorrectMax: 4},
		GridImages: imgs,
	}}
}

// writePlantsPack 在 base 下写入一个含 11 张「植物」图的磁盘素材包。
func writePlantsPack(t *testing.T, base string) {
	t.Helper()
	dir := filepath.Join(base, "plants")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	var imgs strings.Builder
	for i := 1; i <= 11; i++ {
		file := fmt.Sprintf("leaf_%02d.webp", i)
		imgs.WriteString(`{"file":"` + file + `","tags":["植物"]},`)
		if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	meta := `{"pack_name":"植物测试包","tag_defs":{"植物":{"name":"植物"}},` +
		`"grid":{"size":9,"correct_min":2,"correct_max":4},"grid_images":[` +
		strings.TrimSuffix(imgs.String(), ",") + `]}`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}
}

func TestConsoleHelpStatusMetrics(t *testing.T) {
	c, out := consoleTestEnv(t, t.TempDir())

	if !c.exec("help") {
		t.Error("help 不应退出循环")
	}
	for _, want := range []string{"显示帮助", "运行状态", "请求指标", "重新加载素材包", "调整 LOG_LEVEL"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help 输出缺少 %q：%s", want, out.String())
		}
	}

	out.Reset()
	if !c.exec("status") {
		t.Error("status 不应退出循环")
	}
	for _, want := range []string{"时长", ":8080", "素材包", "easy", "日志级别"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status 输出缺少 %q：%s", want, out.String())
		}
	}

	out.Reset()
	if !c.exec("metrics") {
		t.Error("metrics 不应退出循环")
	}
	for _, want := range []string{"挑战生成", "网格图生成", "验证通过", "验证失败", "资源下发"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("metrics 输出缺少 %q：%s", want, out.String())
		}
	}

	out.Reset()
	if !c.exec("bogus") {
		t.Error("未知命令不应退出循环")
	}
	if !strings.Contains(out.String(), "未知命令") {
		t.Errorf("未知命令应给出提示：%s", out.String())
	}
}

func TestConsoleSet(t *testing.T) {
	c, out := consoleTestEnv(t, t.TempDir())
	defer core.SetLogLevel("info")

	if !c.exec("set LOG_LEVEL=debug") {
		t.Error("set 不应退出循环")
	}
	if got := core.LogLevel(); got != "debug" {
		t.Errorf("LogLevel = %q, want debug", got)
	}

	out.Reset()
	if !c.exec("set LOG_LEVEL=bogus") {
		t.Error("非法日志级别不应退出循环")
	}
	if !strings.Contains(out.String(), "必须为 debug / info / warn / error") {
		t.Errorf("非法日志级别应报错：%s", out.String())
	}
	if got := core.LogLevel(); got != "debug" {
		t.Errorf("非法值不应改变日志级别，当前=%q", got)
	}

	out.Reset()
	if !c.exec("set CAPTCHA_DIFFICULTY=hard") {
		t.Error("set 不应退出循环")
	}
	if got := c.service.CurrentDifficulty(); got != core.DiffHard {
		t.Errorf("CurrentDifficulty = %q, want hard", got)
	}

	out.Reset()
	if !c.exec("set CAPTCHA_DIFFICULTY=impossible") {
		t.Error("非法难度不应退出循环")
	}
	if !strings.Contains(out.String(), "必须为 easy / medium / hard") {
		t.Errorf("非法难度应报错：%s", out.String())
	}
	if got := c.service.CurrentDifficulty(); got != core.DiffHard {
		t.Errorf("非法值不应改变难度，当前=%q", got)
	}

	out.Reset()
	if !c.exec("set FOO=1") {
		t.Error("set 不应退出循环")
	}
	if !strings.Contains(out.String(), "仅支持调整 LOG_LEVEL、CAPTCHA_DIFFICULTY") {
		t.Errorf("白名单外键应报错：%s", out.String())
	}

	out.Reset()
	if !c.exec("set") {
		t.Error("缺参 set 不应退出循环")
	}
	if !strings.Contains(out.String(), "用法：set KEY=VALUE") {
		t.Errorf("缺参 set 应显示用法：%s", out.String())
	}
}

// TestConsoleConfig 验证 config 命令展示「本次运行实际生效」的配置及来源：
// 用自定义 Lookup 模拟真实环境变量，确保测试不依赖宿主 shell 的环境。
func TestConsoleConfig(t *testing.T) {
	c, out := consoleTestEnv(t, t.TempDir())
	_, resolved, err := core.Load(core.LoadOptions{
		Lookup: func(key string) (string, bool) {
			switch key {
			case "HTTP_PORT":
				return "9000", true
			case "CAPTCHA_DIFFICULTY":
				return "hard", true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.config = resolved

	if !c.exec("config") {
		t.Error("config 不应退出循环")
	}
	for _, want := range []string{"HTTP_PORT", "9000", "环境变量", "CAPTCHA_DIFFICULTY", "hard"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("config 输出缺少 %q：%s", want, out.String())
		}
	}
	out.Reset()
	if !c.exec("config HTTP_PORT") {
		t.Error("config KEY 不应退出循环")
	}
	if !strings.Contains(out.String(), "HTTP_PORT  9000  环境变量") {
		t.Errorf("config KEY 应输出单项及来源：%s", out.String())
	}

	out.Reset()
	if !c.exec("config CAPTCHA_DIFFICULTY") {
		t.Error("config KEY 不应退出循环")
	}
	if !strings.Contains(out.String(), "CAPTCHA_DIFFICULTY  hard  环境变量") {
		t.Errorf("config KEY 应输出单项及来源：%s", out.String())
	}

	out.Reset()
	if !c.exec("config NOPE") {
		t.Error("config 未知键不应退出循环")
	}
	if !strings.Contains(out.String(), "未知配置项") {
		t.Errorf("config 未知键应报错：%s", out.String())
	}
}

func TestConsoleQuit(t *testing.T) {
	c, _ := consoleTestEnv(t, t.TempDir())
	if c.exec("quit") {
		t.Error("quit 应退出循环")
	}
	if c.exec("exit") {
		t.Error("exit 应退出循环")
	}
}

func TestConsoleReload(t *testing.T) {
	base := t.TempDir()
	c, out := consoleTestEnv(t, base)

	// 初始索引只有 animals（猫）。
	if tags := c.engine.Indexer().GetAllGridTags(); len(tags) != 1 || tags[0] != "猫" {
		t.Fatalf("初始标签 = %v, want [猫]", tags)
	}

	// 写入 plants 素材包后 reload：索引应原子替换为 plants。
	writePlantsPack(t, base)
	if !c.exec("reload") {
		t.Error("reload 不应退出循环")
	}
	if !strings.Contains(out.String(), "已重新加载") {
		t.Errorf("reload 应报告成功：%s", out.String())
	}
	got := c.engine.Indexer().GetAllGridTags()
	if len(got) != 1 || got[0] != "植物" {
		t.Errorf("reload 后标签 = %v, want [植物]", got)
	}
	if cts := c.engine.Indexer().Counts(); cts.Packs != 1 || cts.GridImages != 11 {
		t.Errorf("reload 后 Counts = %+v, want Packs=1 GridImages=11", cts)
	}

	// reload 失败（目录里没有任何有效 pack）时应保留旧素材。
	badBase := t.TempDir()
	c2, out2 := consoleTestEnv(t, badBase)
	if !c2.exec("reload") {
		t.Error("失败 reload 不应退出循环")
	}
	if !strings.Contains(out2.String(), "重新加载失败") {
		t.Errorf("失败 reload 应报错：%s", out2.String())
	}
	if tags := c2.engine.Indexer().GetAllGridTags(); len(tags) != 1 || tags[0] != "猫" {
		t.Errorf("失败 reload 后应保留旧索引，当前标签 = %v", tags)
	}
}

func TestResolveGlyphsFor(t *testing.T) {
	if g := resolveGlyphsFor("nerd", false); g.ok != glyphsNerd.ok {
		t.Errorf("强制 nerd 失效：ok=%q", g.ok)
	}
	if g := resolveGlyphsFor("unicode", false); g.ok != glyphsUnicode.ok {
		t.Errorf("强制 unicode 失效：ok=%q", g.ok)
	}
	if g := resolveGlyphsFor("ascii", true); g.ok != glyphsASCII.ok {
		t.Errorf("强制 ascii 失效：ok=%q", g.ok)
	}
	// auto + 非终端 → 纯 ASCII；auto + 终端但无 Nerd 迹象 → Unicode。
	if g := resolveGlyphsFor("auto", false); g.ok != glyphsASCII.ok {
		t.Errorf("auto 非终端应回落 ascii：ok=%q", g.ok)
	}
	if g := resolveGlyphsFor("", false); g.ok != glyphsASCII.ok {
		t.Errorf("未设置应等同 auto：ok=%q", g.ok)
	}

	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("WEZTERM_EXECUTABLE", "")
	t.Setenv("GHOSTTY_RESOURCES_DIR", "")
	t.Setenv("TERM_PROGRAM", "")
	if g := resolveGlyphsFor("auto", true); g.ok != glyphsUnicode.ok {
		t.Errorf("auto 终端无 Nerd 迹象应回落 unicode：ok=%q", g.ok)
	}
	t.Setenv("KITTY_WINDOW_ID", "1")
	if g := resolveGlyphsFor("auto", true); g.ok != glyphsNerd.ok {
		t.Errorf("kitty 终端应启用 nerd：ok=%q", g.ok)
	}
}
