package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"moetcha/core"
)

const configUsageText = `用法：moetcha config <子命令> [选项]

子命令：
  init      生成 .env（交互向导，或用 --preset dev|prod 非交互生成）
  show      查看生效配置及其来源
  validate  校验配置，有错则以非零码退出
  template  输出完整 .env 模板到标准输出
`

func runConfig(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, configUsageText)
		return 2
	}
	switch args[0] {
	case "init":
		return runConfigInit(args[1:])
	case "show":
		return runConfigShow(args[1:])
	case "validate":
		return runConfigValidate(args[1:])
	case "template":
		return runConfigTemplate(args[1:])
	case "help", "-h", "--help":
		fmt.Print(configUsageText)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知子命令：%s\n\n%s", args[0], configUsageText)
		return 2
	}
}

func runConfigShow(args []string) int {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	envFile := fs.String("env-file", defaultEnvFile, "指定 .env 文件路径")
	format := fs.String("format", "table", "输出格式：table / env / json")
	showSecrets := fs.Bool("show-secrets", false, "明文显示密钥（默认脱敏）")
	var sets multiFlag
	fs.Var(&sets, "set", "覆盖任意配置项，形如 KEY=VALUE，可重复")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	overrides, err := parseSetFlags(sets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}

	// 宽松模式：配置有错时仍完整渲染并逐项标注，便于对照排查。
	_, resolved, err := loadConfig(*envFile, flagWasSet(fs, "env-file"), overrides, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	switch *format {
	case "table":
		printConfigTable(resolved, *showSecrets)
	case "env":
		printConfigEnv(resolved, *showSecrets)
	case "json":
		if err := printConfigJSON(resolved, *showSecrets); err != nil {
			fmt.Fprintf(os.Stderr, "序列化失败：%v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "错误：--format 必须为 table / env / json，当前=%q\n", *format)
		return 2
	}

	if n := countBad(resolved); n > 0 {
		fmt.Fprintf(os.Stderr, "\n发现 %d 项配置无法解析（上表已标注 ✗），这些项当前使用默认值。\n", n)
		return 1
	}
	return 0
}

func displayValue(rv core.ResolvedValue, showSecrets bool) string {
	if showSecrets {
		if rv.Raw == "" {
			return `""`
		}
		return rv.Raw
	}
	return rv.Value
}

func printConfigTable(resolved []core.ResolvedValue, showSecrets bool) {
	const (
		headKey    = "变量"
		headValue  = "值"
		headSource = "来源"
	)

	keyW, valW := displayWidth(headKey), displayWidth(headValue)
	for _, rv := range resolved {
		keyW = max(keyW, displayWidth(rv.Spec.Key))
		valW = max(valW, displayWidth(displayValue(rv, showSecrets)))
	}

	section := ""
	for _, rv := range resolved {
		if rv.Spec.Section != section {
			section = rv.Spec.Section
			fmt.Printf("\n# %s\n", section)
			fmt.Printf("%s  %s  %s\n", pad(headKey, keyW), pad(headValue, valW), headSource)
		}
		line := fmt.Sprintf("%s  %s  %s",
			pad(rv.Spec.Key, keyW), pad(displayValue(rv, showSecrets), valW), rv.Source)
		if rv.Err != nil {
			line += "  ✗ " + rv.Err.Error()
		}
		fmt.Println(line)
	}
}

func pad(s string, w int) string {
	if n := w - displayWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// displayWidth 返回字符串在终端中占用的列数。
// 中日韩字符渲染为双宽，而 text/tabwriter 按字符计数，
// 全中文的表头会因此错位，故自行计算宽度。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWideRune(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // 韩文字母
		r >= 0x2E80 && r <= 0x303E, // 康熙部首、中日韩符号
		r >= 0x3041 && r <= 0x33FF, // 假名、注音、兼容符号
		r >= 0x3400 && r <= 0x4DBF, // 中日韩扩展 A
		r >= 0x4E00 && r <= 0x9FFF, // 中日韩统一表意文字
		r >= 0xA000 && r <= 0xA4CF, // 彝文
		r >= 0xAC00 && r <= 0xD7A3, // 韩文音节
		r >= 0xF900 && r <= 0xFAFF, // 兼容表意文字
		r >= 0xFE30 && r <= 0xFE6F, // 竖排与小写变体
		r >= 0xFF00 && r <= 0xFF60, // 全角字符
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F9FF, // 绘文字
		r >= 0x20000 && r <= 0x2FFFD: // 中日韩扩展 B 及以后
		return true
	}
	return false
}

func printConfigEnv(resolved []core.ResolvedValue, showSecrets bool) {
	if !showSecrets {
		fmt.Println("# 注意：密钥项已脱敏，此输出不能直接当作 .env 使用（需要 --show-secrets）")
	}
	section := ""
	for _, rv := range resolved {
		if rv.Spec.Section != section {
			section = rv.Spec.Section
			fmt.Printf("\n# ==== %s ====\n", section)
		}
		value := rv.Raw
		if !showSecrets && rv.Spec.Secret {
			value = rv.Value
		}
		fmt.Printf("%s=%s\n", rv.Spec.Key, value)
	}
}

type configJSONItem struct {
	Key     string `json:"key"`
	Section string `json:"section"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Source  string `json:"source"`
	Secret  bool   `json:"secret"`
	Error   string `json:"error,omitempty"`
}

func printConfigJSON(resolved []core.ResolvedValue, showSecrets bool) error {
	items := make([]configJSONItem, 0, len(resolved))
	for _, rv := range resolved {
		item := configJSONItem{
			Key:     rv.Spec.Key,
			Section: rv.Spec.Section,
			Kind:    rv.Spec.Kind.String(),
			Value:   displayValue(rv, showSecrets),
			Source:  rv.Source.String(),
			Secret:  rv.Spec.Secret,
		}
		if rv.Err != nil {
			item.Error = rv.Err.Error()
		}
		items = append(items, item)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func countBad(resolved []core.ResolvedValue) int {
	n := 0
	for _, rv := range resolved {
		if rv.Err != nil {
			n++
		}
	}
	return n
}

func runConfigValidate(args []string) int {
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	envFile := fs.String("env-file", defaultEnvFile, "指定 .env 文件路径")
	var sets multiFlag
	fs.Var(&sets, "set", "覆盖任意配置项，形如 KEY=VALUE，可重复")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	overrides, err := parseSetFlags(sets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}

	cfg, _, err := loadConfig(*envFile, flagWasSet(fs, "env-file"), overrides, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := core.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "配置校验失败：%v\n", err)
		return 1
	}

	fmt.Println("✓ 配置校验通过")
	if len(cfg.Service.APIAuth.Tokens) == 0 {
		fmt.Println("提示：API_TOKENS 未配置，/grid/generate 处于开放模式（仅适合内网/本地开发）")
	}
	return 0
}

func runConfigTemplate(args []string) int {
	fs := flag.NewFlagSet("config template", flag.ContinueOnError)
	output := fs.String("output", "-", "输出路径，- 表示标准输出")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	content := core.RenderDotEnvTemplate(nil)
	if *output == "-" {
		fmt.Print(content)
		return 0
	}
	if err := os.WriteFile(*output, []byte(content), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "写入 %s 失败：%v\n", *output, err)
		return 1
	}
	fmt.Printf("✓ 已写入 %s\n", *output)
	return 0
}

func runConfigInit(args []string) int {
	fs := flag.NewFlagSet("config init", flag.ContinueOnError)
	preset := fs.String("preset", "", "使用预设而非交互向导：dev 或 prod")
	output := fs.String("output", defaultEnvFile, "输出路径")
	force := fs.Bool("force", false, "覆盖已存在的文件")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if _, err := os.Stat(*output); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "错误：%s 已存在，如需覆盖请加 --force\n", *output)
		return 1
	}

	var overrides map[string]string
	var err error
	switch *preset {
	case "dev", "prod":
		overrides, err = presetOverrides(*preset)
	case "":
		overrides, err = wizardOverrides()
	default:
		fmt.Fprintf(os.Stderr, "错误：--preset 必须为 dev 或 prod，当前=%q\n", *preset)
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成配置失败：%v\n", err)
		return 1
	}

	// 0600：文件可能含签名密钥与 API Token。
	if err := os.WriteFile(*output, []byte(core.RenderDotEnvTemplate(overrides)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "写入 %s 失败：%v\n", *output, err)
		return 1
	}
	fmt.Printf("✓ 已写入 %s\n", *output)
	if overrides["CAPTCHA_TOKEN_SIGNING_KEY"] != "" || overrides["API_TOKENS"] != "" {
		fmt.Println("该文件含密钥，请勿提交到版本库（.gitignore 已包含 .env）")
	}
	return 0
}

func presetOverrides(preset string) (map[string]string, error) {
	if preset == "dev" {
		return map[string]string{
			"LOG_LEVEL":          "debug",
			"STORAGE_BACKEND":    "memory",
			"CAPTCHA_DIFFICULTY": "easy",
		}, nil
	}

	signingKey, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	apiToken, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"LOG_LEVEL":                    "info",
		"STORAGE_BACKEND":              "sqlite",
		"CAPTCHA_DIFFICULTY":           "medium",
		"CAPTCHA_IP_ENABLED":           "true",
		"CAPTCHA_REQUIRE_UA":           "true",
		"CAPTCHA_TOKEN_ENABLED":        "true",
		"CAPTCHA_TOKEN_SIGNING_KEY":    signingKey,
		"CAPTCHA_RATE_LIMIT_ENABLED":   "true",
		"CAPTCHA_RATE_LIMIT_IP_QPS":    "20",
		"CAPTCHA_RATE_LIMIT_IP_BURST":  "40",
		"CAPTCHA_RATE_LIMIT_BLOCK_TTL": "5m",
		"API_TOKENS":                   apiToken,
	}, nil
}

// wizardOverrides 只问关键项，其余用默认值——问满 38 项没人愿意答完。
func wizardOverrides() (map[string]string, error) {
	p := newPrompter()
	if !p.interactive {
		p.notef("检测到非交互式输入，使用 dev 预设。如需指定，请加 --preset dev|prod。\n")
		return presetOverrides("dev")
	}

	out := map[string]string{}
	p.notef("生成 MoeTcha 配置，直接回车使用中括号内的默认值。\n\n")

	out["HTTP_PORT"] = p.ask("HTTP 端口", defaultOf("HTTP_PORT"))
	out["STORAGE_BACKEND"] = p.askEnum("存储后端", []string{"memory", "sqlite"}, defaultOf("STORAGE_BACKEND"))
	if out["STORAGE_BACKEND"] == "sqlite" {
		out["SQLITE_PATH"] = p.ask("SQLite 路径", defaultOf("SQLITE_PATH"))
	}
	out["CAPTCHA_DIFFICULTY"] = p.askEnum("验证码难度", []string{"easy", "medium", "hard"}, defaultOf("CAPTCHA_DIFFICULTY"))
	out["LOG_LEVEL"] = p.askEnum("日志级别", []string{"debug", "info", "warn", "error"}, defaultOf("LOG_LEVEL"))

	if p.askBool("启用 Token 签名？", false) {
		key, err := randomHex(32)
		if err != nil {
			return nil, err
		}
		out["CAPTCHA_TOKEN_ENABLED"] = "true"
		out["CAPTCHA_TOKEN_SIGNING_KEY"] = key
		p.notef("  → 已自动生成 CAPTCHA_TOKEN_SIGNING_KEY\n")
	}

	if p.askBool("生成 API_TOKENS（保护 /grid/generate）？", true) {
		token, err := randomHex(32)
		if err != nil {
			return nil, err
		}
		out["API_TOKENS"] = token
		p.notef("  → 已自动生成 API_TOKENS\n")
	}

	if p.askBool("启用限流？", false) {
		out["CAPTCHA_RATE_LIMIT_ENABLED"] = "true"
		out["CAPTCHA_RATE_LIMIT_IP_QPS"] = p.ask("  单 IP 每秒请求数上限", "20")
		out["CAPTCHA_RATE_LIMIT_IP_BURST"] = p.ask("  单 IP 突发容量", "40")
	}

	p.notef("\n")
	return out, nil
}

func defaultOf(key string) string {
	if s, ok := core.SpecByKey(key); ok {
		return s.Default
	}
	return ""
}

// coreSpecByKey 供 parseSetFlags 校验 --set 的键名。
func coreSpecByKey(key string) (core.Spec, bool) {
	return core.SpecByKey(strings.TrimSpace(key))
}
