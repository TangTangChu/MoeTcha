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
	"moetcha/core/render"
	httptransport "moetcha/transport/http"
)

const defaultEnvFile = ".env"

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	envFile := fs.String("env-file", defaultEnvFile, "指定 .env 文件路径")
	port := fs.String("port", "", "覆盖 HTTP_PORT")
	logLevel := fs.String("log-level", "", "覆盖 LOG_LEVEL")
	var sets multiFlag
	fs.Var(&sets, "set", "覆盖任意配置项，形如 KEY=VALUE，可重复")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法：moetcha serve [选项]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	overrides, err := parseSetFlags(sets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	overrides = withOverride(overrides, "HTTP_PORT", *port)
	overrides = withOverride(overrides, "LOG_LEVEL", *logLevel)

	cfg, _, err := loadConfig(*envFile, flagWasSet(fs, "env-file"), overrides, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := core.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "配置校验失败：%v\n", err)
		return 1
	}

	core.InitLogger(cfg.LogLevel)
	return serve(cfg)
}

func serve(config core.Config) int {
	provider := &core.DirectoryProvider{
		BaseDir:      "./data/packs",
		MetaFileName: "meta.json",
		Strict:       true,
	}

	packs, err := provider.LoadPacks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载 packs 失败：%v\n", err)
		return 1
	}
	fmt.Printf("加载 pack 数量：%d\n", len(packs))

	indexer, err := core.NewIndexer(provider)
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
		fmt.Printf("存储后端：SQLite (%s)\n", config.SQLitePath)
	default:
		sessionStore = core.NewMemorySessionStore()
		assetStore = core.NewMemoryAssetStore()
		fmt.Println("存储后端：内存")
	}
	defer func() {
		if sqliteStore != nil {
			sqliteStore.Close()
		}
	}()

	renderer := &core.Renderer{
		Pipeline: render.NewPipeline(render.NoiseObfuscator{Density: 0.02}),
	}
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
	}

	// ReleaseMode 抑制 gin 启动时那一串 [GIN-debug] 路由注册输出与
	// "Listening and serving" 行，交由下方自行打印干净的就绪信息。
	// 请求日志由 LoggingMiddleware 独立记录，不受此开关影响。
	httptransport.SetReleaseMode()
	router := httptransport.NewRouter(service, assetStore, httptransport.RouterConfig{
		APIAuth: config.Service.APIAuth,
		CORS:    config.CORS,
	})

	fmt.Printf("难度：%s\n", config.Service.Difficulty)
	if len(config.Service.APIAuth.Tokens) == 0 {
		fmt.Fprintln(os.Stderr, errStyle.yellow("警告：API_TOKENS 未配置，/grid/generate 处于开放模式（仅适合内网/本地开发）"))
	}
	if config.CORS.Enabled && corsAllowsAll(config.CORS.AllowedOrigins) {
		fmt.Fprintln(os.Stderr, errStyle.yellow("警告：CORS_ALLOWED_ORIGINS=* 放行任意来源，生产环境请改为具体域名"))
	}
	fmt.Printf("%s 服务已就绪  ->  http://localhost:%s   （按 Ctrl+C 退出）\n",
		outStyle.green("✓"), config.HTTPPort)

	// 捕获 Ctrl+C / SIGTERM 做优雅关闭：先 Shutdown 停止接收新连接并排空在途
	// 请求，再让 serve() 正常返回——此时外层 defer sqliteStore.Close() 才会执行，
	// SQLite 得以 checkpoint 落盘。直接被信号杀掉则这些 defer 不会跑。
	srv := &http.Server{Addr: ":" + config.HTTPPort, Handler: router.Engine}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		// ListenAndServe 返回：要么启动失败（端口占用等），要么被 Shutdown 关闭。
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "%s HTTP 服务启动失败：%v\n", errStyle.red("✗"), err)
			return 1
		}
	case <-sigCh:
		fmt.Fprintf(os.Stderr, "\n%s 收到退出信号，正在关闭…\n", errStyle.yellow("•"))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "%s 关闭超时：%v\n", errStyle.red("✗"), err)
		}
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
