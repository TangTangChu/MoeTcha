package main

import (
	"fmt"
	"log"
	"time"

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
	sessionStore := core.NewMemorySessionStore()
	assetStore := core.NewMemoryAssetStore()
	renderer := &core.Renderer{
		Pipeline: render.NewPipeline(render.NoiseObfuscator{Density: 0.02}),
	}
	service := &core.Service{
		Engine:       engine,
		SessionStore: sessionStore,
		AssetStore:   assetStore,
		Renderer:     renderer,
		TTL:          2 * time.Minute,
		MaxAttempts:  3,
	}

	router := httptransport.NewRouter(service, assetStore)
	if err := router.Engine.Run(":8080"); err != nil {
		log.Fatalf("HTTP 服务启动失败: %v", err)
	}
}
