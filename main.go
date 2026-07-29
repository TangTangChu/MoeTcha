package main

import (
	"fmt"
	"log"

	"moetcha/core"
	"moetcha/core/render"
	httptransport "moetcha/transport/http"
)

func main() {
	provider := &core.DirectoryProvider{
		BaseDir:      "./data/packs",
		MetaFileName: "meta.json",
		Strict:       true,
	}

	packs, err := provider.LoadPacks()
	if err != nil {
		log.Fatalf("加载 packs 失败：%v", err)
	}
	fmt.Printf("加载 pack 数量：%d\n", len(packs))

	indexer, err := core.NewIndexer(provider)
	if err != nil {
		log.Fatalf("构建索引失败：%v", err)
	}

	engine := core.NewEngine(indexer)

	config, err := core.LoadConfig()
	if err != nil {
		log.Fatalf("配置加载失败：%v", err)
	}
	if err := core.ValidateConfig(config); err != nil {
		log.Fatalf("配置校验失败：%v", err)
	}

	var sessionStore core.SessionStore
	var assetStore core.AssetStore
	var sqliteStore *core.SQLiteSessionStore

	switch config.Storage.Backend {
	case "sqlite":
		ss, err := core.NewSQLiteSessionStore(config.SQLitePath)
		if err != nil {
			log.Fatalf("SQLite 初始化失败：%v", err)
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

	renderer := &core.Renderer{
		Pipeline: render.NewPipeline(render.NoiseObfuscator{Density: 0.02}),
	}
	service := &core.Service{
		Engine:              engine,
		SessionStore:        sessionStore,
		AssetStore:          assetStore,
		Renderer:            renderer,
		TTL:                 config.Service.TTL,
		MaxAttempts:         config.Service.MaxAttempts,
		Difficulty:          config.Service.Difficulty,
		IPPolicy:            config.Service.IPPolicy,
		Secure:              config.Service.Secure,
		GridConcurrency:     config.Service.GridGenerateConcurrency,
		MaxSourcePixels:     config.Service.MaxSourceImagePixels,
	}

	fmt.Printf("难度：%s\n", config.Service.Difficulty)
	if len(config.Service.APIAuth.Tokens) == 0 {
		fmt.Println("警告：API_TOKENS 未配置，/grid/generate 处于开放模式（仅适合内网/本地开发）")
	}

	router := httptransport.NewRouter(service, assetStore, config.Service.APIAuth)
	if err := router.Engine.Run(":" + config.HTTPPort); err != nil {
		if sqliteStore != nil {
			sqliteStore.Close()
		}
		log.Fatalf("HTTP 服务启动失败: %v", err)
	}
}
