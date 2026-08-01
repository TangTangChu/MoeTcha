// 运行期控制台：serve 运行期间从 stdin 读取管理命令。
//
// 默认仅当 stdin 是终端时启用（防止管道/容器里误读数据流），可用
// `serve --console on` 强制开启（如 docker run -i）、`--console off` 关闭。
// 输出一律写 stderr——stdout 是 JSON 日志的专属通道，混入控制台文本会污染
// 日志管道。命令全部无锁安全：reload 走引擎的原子索引替换，set 走 logger /
// service 的原子槽。
//
// 排版约定：提示符即输入标记，所有输出行统一缩进 2 格挂在提示符下，命令
// 结束后空一行再出下一个提示符——输入与输出一眼可分。
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"moetcha/core"
)

// console 是 serve 运行期间的命令台。quit 由 serve 注入：执行优雅关闭
// （等价于 Ctrl+C），通过 ListenAndServe 返回 ErrServerClosed 让主流程收尾。
type console struct {
	out       io.Writer
	engine    *core.Engine
	service   *core.Service
	provider  *core.DirectoryProvider
	port      string
	backend   string
	sqliteDir string
	started   time.Time
	quit      func()
	// config 是本进程启动时解析出的生效配置（含来源），由 serve 注入；
	// config 命令据此展示「这次运行实际用了什么」，而非当前 shell 的环境。
	config []core.ResolvedValue
}

func newConsole(engine *core.Engine, service *core.Service, provider *core.DirectoryProvider, port, backend, sqlitePath string) *console {
	return &console{
		out:       os.Stderr,
		engine:    engine,
		service:   service,
		provider:  provider,
		port:      port,
		backend:   backend,
		sqliteDir: sqlitePath,
		started:   time.Now(),
	}
}

// line 输出一行命令结果，统一缩进 2 格，与提示符对齐。
func (c *console) line(format string, a ...any) {
	fmt.Fprintf(c.out, "  "+format+"\n", a...)
}

// prefixWriter 给写入的每一行加前缀，用于把多行表格也缩进到提示符下。
type prefixWriter struct {
	w      io.Writer
	prefix string
}

func (p prefixWriter) Write(b []byte) (int, error) {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if i == len(lines)-1 && l == "" {
			continue // 末尾换行符产生的空串
		}
		if _, err := io.WriteString(p.w, p.prefix+l+"\n"); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

// loop 运行命令循环；stdin 关闭（Ctrl+D / 容器 detach）或 quit 时返回，
// 服务本身继续运行。调用方应在独立 goroutine 中启动。
func (c *console) loop() {
	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(c.out, glyphs.prompt+" ")
		if !sc.Scan() {
			return
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			fmt.Fprintln(c.out)
			continue
		}
		if !c.exec(line) {
			return
		}
		fmt.Fprintln(c.out) // 输出块与下一条命令之间空一行
	}
}

// exec 执行一条命令，返回是否继续运行（quit 返回 false）。
func (c *console) exec(line string) bool {
	fields := strings.Fields(line)
	cmd, args := fields[0], fields[1:]
	switch cmd {
	case "help", "?":
		c.cmdHelp()
	case "status":
		c.cmdStatus()
	case "metrics":
		c.cmdMetrics()
	case "config":
		c.cmdConfig(args)
	case "reload":
		c.cmdReload()
	case "set":
		c.cmdSet(args)
	case "quit", "exit", "q":
		if c.quit != nil {
			c.quit()
		}
		return false
	default:
		c.line("%s 未知命令：%s（help 查看命令）", glyphs.fail, cmd)
	}
	return true
}

func (c *console) cmdHelp() {
	c.line("help    显示帮助")
	c.line("status  运行状态")
	c.line("metrics 请求指标")
	c.line("config  查看生效配置及来源（config KEY 只看单项）")
	c.line("reload  重新加载素材包")
	c.line("set K=V 调整 LOG_LEVEL / CAPTCHA_DIFFICULTY")
	c.line("quit    退出（等价 Ctrl+C）")
}

func (c *console) cmdStatus() {
	cts := c.engine.Indexer().Counts()
	m := core.MetricsInstance.Snapshot()
	storage := c.backend
	if c.backend == "sqlite" && c.sqliteDir != "" {
		storage = fmt.Sprintf("sqlite (%s)", c.sqliteDir)
	}
	c.line("%s %s", pad("时长", 8), time.Since(c.started).Round(time.Second))
	c.line("%s :%s", pad("监听", 8), c.port)
	c.line("%s %s", pad("存储", 8), storage)
	c.line("%s %d 个（Grid %d 图 / %d 标签，Click %d 图 / %d 标签）",
		pad("素材包", 8), cts.Packs, cts.GridImages, cts.GridTags, cts.ClickImages, cts.ClickTags)
	c.line("%s %s", pad("难度", 8), c.service.CurrentDifficulty())
	c.line("%s %s", pad("日志级别", 8), core.LogLevel())
	c.line("%s 挑战 %d / 网格图 %d / 通过 %d / 失败 %d / 资源 %d",
		pad("指标", 8), m["challenges_generated"], m["grid_images_generated"],
		m["verifications_ok"], m["verifications_fail"], m["assets_served"])
}

func (c *console) cmdMetrics() {
	m := core.MetricsInstance.Snapshot()
	c.line("%s %d", pad("挑战生成", 10), m["challenges_generated"])
	c.line("%s %d", pad("网格图生成", 10), m["grid_images_generated"])
	c.line("%s %d", pad("验证通过", 10), m["verifications_ok"])
	c.line("%s %d", pad("验证失败", 10), m["verifications_fail"])
	c.line("%s %d", pad("资源下发", 10), m["assets_served"])
}

// cmdConfig 展示本进程启动时解析出的生效配置及来源（默认值 / .env 文件 /
// 环境变量 / 命令行），密钥默认脱敏。config KEY 只显示单项。
func (c *console) cmdConfig(args []string) {
	if len(args) > 1 {
		c.line("用法：config [KEY]")
		return
	}
	if len(args) == 1 {
		key := strings.ToUpper(strings.TrimSpace(args[0]))
		for _, rv := range c.config {
			if rv.Spec.Key == key {
				line := fmt.Sprintf("%s  %s  %s", rv.Spec.Key, displayValue(rv, false), rv.Source)
				if rv.Err != nil {
					line += "  " + outStyle.red(glyphs.fail) + " " + rv.Err.Error()
				}
				c.line("%s", line)
				return
			}
		}
		c.line("%s 未知配置项：%s（config 查看全部）", glyphs.fail, key)
		return
	}
	printConfigTable(prefixWriter{w: c.out, prefix: "  "}, c.config, false)
}

func (c *console) cmdReload() {
	packs, err := c.provider.LoadPacks()
	if err != nil {
		c.line("%s 重新加载失败（保留当前素材）：%v", glyphs.fail, err)
		return
	}
	idx, err := core.BuildIndexer(packs)
	if err != nil {
		c.line("%s 重新加载失败（保留当前素材）：%v", glyphs.fail, err)
		return
	}
	c.engine.SetIndexer(idx)
	cts := idx.Counts()
	c.line("%s 已重新加载 %d 个素材包（Grid %d 图 / %d 标签，Click %d 图 / %d 标签）",
		glyphs.ok, cts.Packs, cts.GridImages, cts.GridTags, cts.ClickImages, cts.ClickTags)
}

// cmdSet 运行期调整白名单配置。其余配置项在启动时烘焙进 Service，
// 运行期修改需要重启才能生效，因此这里只放原子安全的少量旋钮。
func (c *console) cmdSet(args []string) {
	if len(args) != 1 || !strings.Contains(args[0], "=") {
		c.line("用法：set KEY=VALUE（支持 LOG_LEVEL、CAPTCHA_DIFFICULTY）")
		return
	}
	eq := strings.IndexByte(args[0], '=')
	key := strings.TrimSpace(args[0][:eq])
	value := strings.TrimSpace(args[0][eq+1:])
	switch key {
	case "LOG_LEVEL":
		if err := core.SetLogLevel(value); err != nil {
			c.line("%s %v", glyphs.fail, err)
			return
		}
		c.line("%s 日志级别已调整为 %s", glyphs.ok, core.LogLevel())
	case "CAPTCHA_DIFFICULTY":
		d := core.Difficulty(strings.ToLower(value))
		switch d {
		case core.DiffEasy, core.DiffMedium, core.DiffHard:
			c.service.SetDifficulty(d)
			c.line("%s 验证码难度已调整为 %s", glyphs.ok, d)
		default:
			c.line("%s CAPTCHA_DIFFICULTY 必须为 easy / medium / hard，当前=%q", glyphs.fail, value)
		}
	default:
		c.line("%s 运行期仅支持调整 LOG_LEVEL、CAPTCHA_DIFFICULTY；其余配置请改 .env 后重启", glyphs.fail)
	}
}
