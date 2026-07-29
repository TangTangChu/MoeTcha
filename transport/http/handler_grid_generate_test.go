package http

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"moetcha/core"
	"moetcha/core/render"
)

func TestGridGenerateRouteReturnsTemporaryWebPAsset(t *testing.T) {
	packRoot := t.TempDir()
	packDir := filepath.Join(packRoot, "test")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}

	gridImages := make([]map[string]any, 0, 8)
	for i := 1; i <= 4; i++ {
		name := "cat_0" + string(rune('0'+i)) + ".png"
		writeRouteTestPNG(t, filepath.Join(packDir, name), color.RGBA{R: uint8(220 - i*20), A: 255})
		gridImages = append(gridImages, map[string]any{"file": name, "tags": []string{"猫"}})
	}
	for i := 1; i <= 4; i++ {
		name := "dog_0" + string(rune('0'+i)) + ".png"
		writeRouteTestPNG(t, filepath.Join(packDir, name), color.RGBA{B: uint8(220 - i*20), A: 255})
		gridImages = append(gridImages, map[string]any{"file": name, "tags": []string{"狗"}})
	}
	meta := map[string]any{
		"pack_name":   "route test",
		"tag_defs":    map[string]any{"猫": map[string]any{"name": "猫"}, "狗": map[string]any{"name": "狗"}},
		"grid":        map[string]any{"size": 4, "correct_min": 2, "correct_max": 2},
		"grid_images": gridImages,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "meta.json"), metaBytes, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	provider := &core.DirectoryProvider{BaseDir: packRoot, MetaFileName: "meta.json", Strict: true}
	idx, err := core.NewIndexer(provider)
	if err != nil {
		t.Fatalf("NewIndexer: %v", err)
	}
	store := core.NewMemoryAssetStore()
	service := &core.Service{
		Engine:     core.NewEngine(idx),
		AssetStore: store,
		Renderer:   &core.Renderer{Pipeline: render.NewPipeline()},
		TTL:        time.Minute,
		Difficulty: core.DiffEasy,
	}
	router := NewRouter(service, store, core.APIAuthConfig{})

	body := `{"tag":"猫","image_count":4,"correct_count":2,"rows":2,"columns":2,"tile_width":32,"tile_height":32,"gap":2,"padding":2,"apply_renderer":false,"seed":9}`
	req := httptest.NewRequest("POST", "http://example.test/grid/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.Engine.ServeHTTP(resp, req)
	if resp.Code != 200 {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var result core.GridImageGenerateResult
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, resp.Body.String())
	}
	if result.ContentType != "image/webp" || result.AssetKey == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.HasPrefix(result.AssetURL, "http://example.test/asset/") {
		t.Fatalf("asset_url=%q", result.AssetURL)
	}

	assetReq := httptest.NewRequest("GET", "/asset/"+result.AssetKey, nil)
	assetResp := httptest.NewRecorder()
	router.Engine.ServeHTTP(assetResp, assetReq)
	if assetResp.Code != 200 {
		t.Fatalf("asset status=%d body=%s", assetResp.Code, assetResp.Body.String())
	}
	if got := assetResp.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/webp") {
		t.Fatalf("asset content type=%q", got)
	}
	if !bytes.HasPrefix(assetResp.Body.Bytes(), []byte("RIFF")) {
		t.Fatalf("asset is not RIFF WebP")
	}
}

func writeRouteTestPNG(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("encode %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func TestGridGenerateRequiresAPITokenWhenConfigured(t *testing.T) {
	packRoot := t.TempDir()
	packDir := filepath.Join(packRoot, "test")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	gridImages := []map[string]any{
		{"file": "cat_01.png", "tags": []string{"猫"}},
		{"file": "cat_02.png", "tags": []string{"猫"}},
		{"file": "dog_01.png", "tags": []string{"狗"}},
		{"file": "dog_02.png", "tags": []string{"狗"}},
	}
	for _, gi := range gridImages {
		writeRouteTestPNG(t, filepath.Join(packDir, gi["file"].(string)), color.RGBA{A: 255})
	}
	meta := map[string]any{
		"pack_name":   "auth test",
		"tag_defs":    map[string]any{"猫": map[string]any{"name": "猫"}, "狗": map[string]any{"name": "狗"}},
		"grid":        map[string]any{"size": 4, "correct_min": 1, "correct_max": 2},
		"grid_images": gridImages,
	}
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(packDir, "meta.json"), metaBytes, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	provider := &core.DirectoryProvider{BaseDir: packRoot, MetaFileName: "meta.json", Strict: true}
	idx, err := core.NewIndexer(provider)
	if err != nil {
		t.Fatalf("NewIndexer: %v", err)
	}
	store := core.NewMemoryAssetStore()
	service := &core.Service{
		Engine:     core.NewEngine(idx),
		AssetStore: store,
		Renderer:   &core.Renderer{Pipeline: render.NewPipeline()},
		TTL:        time.Minute,
		Difficulty: core.DiffEasy,
	}
	token := "test-secret-token-1234"
	router := NewRouter(service, store, core.APIAuthConfig{Tokens: []string{token}})

	body := `{"tag":"猫","image_count":4,"correct_count":2,"rows":2,"columns":2,"tile_width":16,"tile_height":16,"apply_renderer":false,"seed":1}`

	// 缺 token：401
	req := httptest.NewRequest("POST", "http://example.test/grid/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.Engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status=%d body=%s", resp.Code, resp.Body.String())
	}

	// 错误 token：401
	req = httptest.NewRequest("POST", "http://example.test/grid/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp = httptest.NewRecorder()
	router.Engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-token status=%d", resp.Code)
	}

	// 正确 Bearer：200
	req = httptest.NewRequest("POST", "http://example.test/grid/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp = httptest.NewRecorder()
	router.Engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("valid-token status=%d body=%s", resp.Code, resp.Body.String())
	}

	// 正确 X-API-Token：200
	req = httptest.NewRequest("POST", "http://example.test/grid/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Token", token)
	resp = httptest.NewRecorder()
	router.Engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("x-api-token status=%d body=%s", resp.Code, resp.Body.String())
	}
}
