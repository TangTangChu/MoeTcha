package cli

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"moetcha/core"
	httptransport "moetcha/transport/http"
)

const defaultEnvFile = ".env"

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	envFile := fs.String("env-file", defaultEnvFile, "指定 .env 文件路径")
	port := fs.String("port", "", "覆盖 HTTP_PORT")
	logLevel := fs.String("log-level", "", "覆盖 LOG_LEVEL")
	consoleMode := fs.String("console", "auto", "运行期控制台：auto（stdin 为终端时启用）/ on（强制）/ off（关闭）")
	var sets multiFlag
	fs.Var(&sets, "set", "覆盖任意配置项，形如 KEY=VALUE，可重复")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法：moetcha serve [选项]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	switch *consoleMode {
	case "auto", "on", "off":
	default:
		fmt.Fprintf(os.Stderr, "错误：--console 必须为 auto / on / off，当前=%q\n", *consoleMode)
		return 2
	}

	overrides, err := parseSetFlags(sets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	overrides = withOverride(overrides, "HTTP_PORT", *port)
	overrides = withOverride(overrides, "LOG_LEVEL", *logLevel)

	cfg, resolved, err := loadConfig(*envFile, flagWasSet(fs, "env-file"), overrides, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := core.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "配置校验失败：%v\n", err)
		return 1
	}

	core.InitLogger(cfg.LogLevel)
	return serve(cfg, *consoleMode, resolved)
}

func serve(config core.Config, consoleMode string, resolved []core.ResolvedValue) int {
	provider := &core.DirectoryProvider{
		BaseDir:      "./data/packs",
		MetaFileName: "meta.json",
		Strict:       true,
	}

	// 只扫一次磁盘：LoadPacks 之后直接 BuildIndexer，不再让 NewIndexer 重复读盘。
	packs, err := provider.LoadPacks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载 packs 失败：%v\n", err)
		return 1
	}
	indexer, err := core.BuildIndexer(packs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构建索引失败：%v\n", err)
		return 1
	}

	engine := core.NewEngine(indexer)

	var sessionStore core.SessionStore
	var assetStore core.AssetStore
	var sqliteStore *core.SQLiteSessionStore

	switch config.Storage.Backend {
	case "sqlite":
		ss, err := core.NewSQLiteSessionStore(config.SQLitePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SQLite 初始化失败：%v\n", err)
			return 1
		}
		sqliteStore = ss
		sessionStore = ss
		assetStore = core.NewSQLiteAssetStore(ss.DB())
	default:
		sessionStore = core.NewMemorySessionStore()
		assetStore = core.NewMemoryAssetStore()
	}
	defer func() {
		if sqliteStore != nil {
			sqliteStore.Close()
		}
	}()

	renderer := core.NewRenderer(config.Render)
	service := &core.Service{
		Engine:          engine,
		SessionStore:    sessionStore,
		AssetStore:      assetStore,
		Renderer:        renderer,
		TTL:             config.Service.TTL,
		MaxAttempts:     config.Service.MaxAttempts,
		Difficulty:      config.Service.Difficulty,
		IPPolicy:        config.Service.IPPolicy,
		Secure:          config.Service.Secure,
		GridConcurrency: config.Service.GridGenerateConcurrency,
		MaxSourcePixels: config.Service.MaxSourceImagePixels,
		RenderQuality:   config.Render.Quality,
		GridWebPMethod:  config.Service.GridWebPMethod,
	}

	// ReleaseMode 抑制 gin 启动时那一串 [GIN-debug] 路由注册输出与
	// "Listening and serving" 行，交由下方自行打印干净的就绪信息。
	// 请求日志由 LoggingMiddleware 独立记录，不受此开关影响。
	httptransport.SetReleaseMode()
	router := httptransport.NewRouter(service, assetStore, httptransport.RouterConfig{
		APIAuth: config.Service.APIAuth,
		CORS:    config.CORS,
	})

	// 控制台默认在 stdin 为终端时启用；docker run -i 等场景用 --console on 强制。
	console := newConsole(engine, service, provider, config.HTTPPort, config.Storage.Backend, config.SQLitePath)
	console.config = resolved
	consoleEnabled := consoleMode == "on" || (consoleMode == "auto" && isInteractive())

	cts := indexer.Counts()
	storage := "内存"
	if config.Storage.Backend == "sqlite" {
		storage = "SQLite (" + config.SQLitePath + ")"
	}
	fmt.Printf("  %s %d 个（Grid %d 图 / %d 标签，Click %d 图 / %d 标签）\n",
		pad("素材包", 6), len(packs), cts.GridImages, cts.GridTags, cts.ClickImages, cts.ClickTags)
	fmt.Printf("  %s %s\n", pad("存储", 6), storage)
	fmt.Printf("  %s %s\n", pad("难度", 6), config.Service.Difficulty)
	fmt.Printf("  %s http://localhost:%s\n", pad("监听", 6), config.HTTPPort)
	if consoleEnabled {
		fmt.Printf("  %s 已启用\n", pad("控制台", 6))
	} else {
		fmt.Printf("  %s 未启用（--console on 可强制开启）\n", pad("控制台", 6))
	}
	if len(config.Service.APIAuth.Tokens) == 0 {
		fmt.Fprintln(os.Stderr, "  "+errStyle.yellow("警告：API_TOKENS 未配置，/grid/generate 处于开放模式（仅适合内网/本地开发）"))
	}
	if config.CORS.Enabled && corsAllowsAll(config.CORS.AllowedOrigins) {
		fmt.Fprintln(os.Stderr, "  "+errStyle.yellow("警告：CORS_ALLOWED_ORIGINS=* 放行任意来源，生产环境请改为具体域名"))
	}
	ready := fmt.Sprintf("%s 服务已就绪  %s http://localhost:%s",
		outStyle.green(glyphs.ok), glyphs.arrow, config.HTTPPort)
	if consoleEnabled {
		ready += "（输入 help 查看命令）"
	}
	fmt.Println(ready)

	// 捕获 Ctrl+C / SIGTERM 做优雅关闭：先 Shutdown 停止接收新连接并排空在途
	// 请求，再让 serve() 正常返回——此时外层 defer sqliteStore.Close() 才会执行，
	// SQLite 得以 checkpoint 落盘。直接被信号杀掉则这些 defer 不会跑。
	srv := &http.Server{Addr: ":" + config.HTTPPort, Handler: router.Engine}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	// 优雅关闭：信号与控制台 quit 共用同一条路径。Shutdown 返回后 ListenAndServe
	// 以 ErrServerClosed 退出，主流程从 errCh 收到并正常收尾（defer 关 SQLite）。
	shutdown := func(reason string) {
		fmt.Fprintf(os.Stderr, "\n%s %s，正在关闭…\n", errStyle.yellow(glyphs.warn), reason)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "%s 关闭超时：%v\n", errStyle.red(glyphs.fail), err)
		}
	}
	console.quit = func() { shutdown("收到控制台 quit 命令") }

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	if consoleEnabled {
		go console.loop()
	}

	select {
	case err := <-errCh:
		// ListenAndServe 返回：要么启动失败（端口占用等），要么被 Shutdown 关闭。
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "%s HTTP 服务启动失败：%v\n", errStyle.red(glyphs.fail), err)
			return 1
		}
	case <-sigCh:
		shutdown("收到退出信号")
	}
	return 0
}

// loadConfig 加载 .env（可选）并解析配置。
// mustExist 为真时文件缺失即报错——用户显式指定了 --env-file 却拼错路径，
// 静默忽略会让人误以为配置已生效。
func loadConfig(envFile string, mustExist bool, overrides map[string]string, lenient bool) (core.Config, []core.ResolvedValue, error) {
	if mustExist {
		if _, err := os.Stat(envFile); err != nil {
			return core.Config{}, nil, fmt.Errorf("无法读取配置文件 %s：%w", envFile, err)
		}
	}
	entries, err := core.LoadDotEnvFile(envFile)
	if err != nil {
		return core.Config{}, nil, fmt.Errorf("解析 %s 失败：%w", envFile, err)
	}
	return core.Load(core.LoadOptions{
		DotEnv:  core.DotEnvMap(entries),
		Flags:   overrides,
		Lenient: lenient,
	})
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	seen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func corsAllowsAll(origins []string) bool {
	for _, o := range origins {
		if strings.TrimSpace(o) == "*" {
			return true
		}
	}
	return false
}

func withOverride(m map[string]string, key, value string) map[string]string {
	if value == "" {
		return m
	}
	if m == nil {
		m = map[string]string{}
	}
	m[key] = value
	return m
}
