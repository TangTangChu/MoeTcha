package core

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	genwebp "github.com/gen2brain/webp"
	"moetcha/core/render"
)

func buildGridImageTestIndexer(t *testing.T) (*Indexer, string) {
	t.Helper()
	dir := t.TempDir()
	images := []struct {
		id   string
		tags []string
		c    color.RGBA
	}{
		{id: "cat_01", tags: []string{"猫"}, c: color.RGBA{R: 220, A: 255}},
		{id: "cat_02", tags: []string{"猫"}, c: color.RGBA{R: 180, G: 40, A: 255}},
		{id: "cat_03", tags: []string{"猫"}, c: color.RGBA{R: 140, G: 20, B: 20, A: 255}},
		{id: "cat_04", tags: []string{"猫"}, c: color.RGBA{R: 100, G: 20, B: 20, A: 255}},
		{id: "dog_01", tags: []string{"狗"}, c: color.RGBA{B: 220, A: 255}},
		{id: "dog_02", tags: []string{"狗"}, c: color.RGBA{B: 180, G: 40, A: 255}},
		{id: "bird_01", tags: []string{"鸟"}, c: color.RGBA{G: 220, A: 255}},
		{id: "bird_02", tags: []string{"鸟"}, c: color.RGBA{G: 180, B: 40, A: 255}},
	}
	grid := make([]GridImageMeta, 0, len(images))
	for _, item := range images {
		name := item.id + ".png"
		path := filepath.Join(dir, name)
		img := image.NewRGBA(image.Rect(0, 0, 24, 16))
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				img.SetRGBA(x, y, item.c)
			}
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("create image: %v", err)
		}
		if err := png.Encode(file, img); err != nil {
			file.Close()
			t.Fatalf("encode image: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close image: %v", err)
		}
		grid = append(grid, GridImageMeta{ID: item.id, File: name, Tags: item.tags, PackID: "test", Path: path})
	}

	idx, err := NewIndexer(&mockPackProvider{packs: []Pack{{
		ID:       "test",
		PackName: "grid image test",
		TagDefs: map[string]TagDef{
			"猫": {Name: "猫"},
			"狗": {Name: "狗"},
			"鸟": {Name: "鸟"},
		},
		Grid:       &GridConfig{Size: 4, CorrectMin: 1, CorrectMax: 2},
		GridImages: grid,
	}}})
	if err != nil {
		t.Fatalf("NewIndexer: %v", err)
	}
	return idx, dir
}

func TestBuildGridImagePlanHonorsCorrectTag(t *testing.T) {
	idx, _ := buildGridImageTestIndexer(t)
	engine := NewEngine(idx)
	plan, err := engine.buildGridImagePlan(GridImageGenerateRequest{
		Tag:          "猫",
		ImageCount:   4,
		CorrectCount: 2,
		Rows:         2,
		Columns:      2,
		Shuffle:      boolPtr(false),
		Seed:         int64Ptr(42),
	}, DiffEasy)
	if err != nil {
		t.Fatalf("buildGridImagePlan: %v", err)
	}
	if len(plan.tiles) != 4 {
		t.Fatalf("tiles=%d, want 4", len(plan.tiles))
	}
	if len(plan.correctNumbers) != 2 {
		t.Fatalf("correct numbers=%v, want 2", plan.correctNumbers)
	}
	for i, tile := range plan.tiles {
		if tile.correct != (i < 2) {
			t.Errorf("tile %d correct=%v", i+1, tile.correct)
		}
		if tile.correct && !hasTag(tile.meta.Tags, "猫") {
			t.Errorf("correct tile %d does not contain 猫", i+1)
		}
		if !tile.correct && hasTag(tile.meta.Tags, "猫") {
			t.Errorf("distractor tile %d contains 猫", i+1)
		}
	}
}

func TestBuildGridImagePlanSupportsExplicitNumbers(t *testing.T) {
	idx, _ := buildGridImageTestIndexer(t)
	engine := NewEngine(idx)
	plan, err := engine.buildGridImagePlan(GridImageGenerateRequest{
		Tag:            "猫",
		ImageIDs:       []string{"test:cat_01", "test:dog_01", "test:cat_02", "test:bird_01"},
		CorrectNumbers: []int{1, 3},
		Shuffle:        boolPtr(false),
		Rows:           2,
		Columns:        2,
		TileWidth:      32,
		TileHeight:     32,
	}, DiffEasy)
	if err != nil {
		t.Fatalf("buildGridImagePlan: %v", err)
	}
	if got, want := plan.correctNumbers, []int{1, 3}; !equalInts(got, want) {
		t.Fatalf("correct numbers=%v, want %v", got, want)
	}
	if plan.tiles[0].meta.ID != "cat_01" || plan.tiles[2].meta.ID != "cat_02" {
		t.Fatalf("explicit image order was not preserved")
	}
}

func TestBuildGridImagePlanRejectsCorrectImageWithoutTag(t *testing.T) {
	idx, _ := buildGridImageTestIndexer(t)
	engine := NewEngine(idx)
	_, err := engine.buildGridImagePlan(GridImageGenerateRequest{
		Tag:             "猫",
		ImageCount:      4,
		CorrectImageIDs: []string{"test:dog_01"},
		Seed:            int64Ptr(1),
	}, DiffEasy)
	if err == nil || !IsGridImageRequestError(err) {
		t.Fatalf("expected request error, got %v", err)
	}
}

func TestBuildGridImagePlanSeedIsReproducible(t *testing.T) {
	idx, _ := buildGridImageTestIndexer(t)
	request := GridImageGenerateRequest{
		Tag:          "猫",
		ImageCount:   4,
		CorrectCount: 2,
		Seed:         int64Ptr(1234),
	}
	first, err := NewEngine(idx).buildGridImagePlan(request, DiffEasy)
	if err != nil {
		t.Fatalf("first plan: %v", err)
	}
	second, err := NewEngine(idx).buildGridImagePlan(request, DiffEasy)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	for i := range first.tiles {
		if got, want := globalGridImageID(first.tiles[i].meta), globalGridImageID(second.tiles[i].meta); got != want {
			t.Fatalf("tile %d differs for same seed: %s vs %s", i+1, got, want)
		}
	}
}

func TestServiceGenerateGridImageStoresWebP(t *testing.T) {
	idx, _ := buildGridImageTestIndexer(t)
	store := NewMemoryAssetStore()
	service := &Service{
		Engine:     NewEngine(idx),
		AssetStore: store,
		Renderer:   &Renderer{Pipeline: render.NewPipeline()},
		TTL:        time.Minute,
		Difficulty: DiffEasy,
	}
	result, err := service.GenerateGridImage(GridImageGenerateRequest{
		Tag:                 "猫",
		ImageCount:          4,
		CorrectCount:        2,
		Rows:                2,
		Columns:             2,
		TileWidth:           32,
		TileHeight:          32,
		Gap:                 2,
		Padding:             2,
		ApplyRenderer:       boolPtr(false),
		TemporaryTTLSeconds: 30,
		Seed:                int64Ptr(7),
	}, VerifyContext{})
	if err != nil {
		t.Fatalf("GenerateGridImage: %v", err)
	}
	if result.ContentType != "image/webp" {
		t.Fatalf("content type=%q, want image/webp", result.ContentType)
	}
	if result.Width != 70 || result.Height != 70 {
		t.Fatalf("dimensions=%dx%d, want 70x70", result.Width, result.Height)
	}
	asset, ok := store.Get(result.AssetKey)
	if !ok {
		t.Fatalf("asset %q not found", result.AssetKey)
	}
	if len(asset.Bytes) < 12 || string(asset.Bytes[:4]) != "RIFF" || string(asset.Bytes[8:12]) != "WEBP" {
		t.Fatalf("stored bytes do not have WebP signature")
	}
	if _, err := genwebp.Decode(bytes.NewReader(asset.Bytes)); err != nil {
		t.Fatalf("stored WebP cannot be decoded: %v", err)
	}
	if result.ExpiresAt.Before(result.CreatedAt) {
		t.Fatalf("expires_at=%v before created_at=%v", result.ExpiresAt, result.CreatedAt)
	}
	if len(result.Tiles) != 4 || len(result.CorrectNumbers) != 2 {
		t.Fatalf("unexpected result metadata: tiles=%d correct=%v", len(result.Tiles), result.CorrectNumbers)
	}
}

func TestLoadImageReadsWebPWithoutBuildTag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.webp")
	src := image.NewRGBA(image.Rect(0, 0, 12, 9))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetRGBA(x, y, color.RGBA{R: 20, G: 80, B: 160, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := genwebp.Encode(&encoded, src, genwebp.Options{Quality: 80}); err != nil {
		t.Fatalf("encode source WebP: %v", err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o644); err != nil {
		t.Fatalf("write source WebP: %v", err)
	}
	decoded, err := render.LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	if got := decoded.Bounds().Size(); got != image.Pt(12, 9) {
		t.Fatalf("decoded size=%v, want (12,9)", got)
	}
}

func boolPtr(v bool) *bool { return &v }

func int64Ptr(v int64) *int64 { return &v }

func equalInts(a, b []int) bool {
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

func TestAutoLabelScale(t *testing.T) {
	cases := []struct {
		tileMin, want int
	}{
		{0, 3},       // 退化默认
		{16, 3},      // 最小 tile 仍用下限 3
		{160, 3},     // 160px 默认 -> 3（保持观感）
		{320, 6},     // 2x -> 6
		{600, 11},    // 大 tile -> 11，编号随 tile 变大
		{1200, 22},   // 更大 -> 22
		{2048, 38},   // tile 上限 -> 38
		{100000, 48}, // 超大被钳到 48 上限
	}
	for _, c := range cases {
		if got := autoLabelScale(c.tileMin); got != c.want {
			t.Errorf("autoLabelScale(%d)=%d, want %d", c.tileMin, got, c.want)
		}
	}
}

func TestGridImageLabelScaleAutoFromTileSize(t *testing.T) {
	idx, _ := buildGridImageTestIndexer(t)
	engine := NewEngine(idx)
	plan, err := engine.buildGridImagePlan(GridImageGenerateRequest{
		Tag:          "猫",
		ImageCount:   4,
		CorrectCount: 2,
		Rows:         2,
		Columns:      2,
		TileWidth:    600,
		TileHeight:   600,
		Shuffle:      boolPtr(false),
		Seed:         int64Ptr(7),
	}, DiffEasy)
	if err != nil {
		t.Fatalf("buildGridImagePlan: %v", err)
	}
	if plan.compose.LabelScale != 11 {
		t.Fatalf("auto label_scale for 600px tile=%d, want 11", plan.compose.LabelScale)
	}
}

func TestGridImageLabelScaleExplicitOverridesAuto(t *testing.T) {
	idx, _ := buildGridImageTestIndexer(t)
	engine := NewEngine(idx)
	plan, err := engine.buildGridImagePlan(GridImageGenerateRequest{
		Tag:          "猫",
		ImageCount:   4,
		CorrectCount: 2,
		Rows:         2,
		Columns:      2,
		TileWidth:    600,
		TileHeight:   600,
		LabelScale:   2,
		Shuffle:      boolPtr(false),
		Seed:         int64Ptr(7),
	}, DiffEasy)
	if err != nil {
		t.Fatalf("buildGridImagePlan: %v", err)
	}
	if plan.compose.LabelScale != 2 {
		t.Fatalf("explicit label_scale=2 overridden to %d", plan.compose.LabelScale)
	}
}

func TestGridWebPMethodAuto(t *testing.T) {
	cases := []struct {
		configured, pixels, want int
	}{
		{0, 100_000, 4},    // auto: 小图 -> 4
		{0, 250_000, 4},    // auto: 边界
		{0, 1_000_000, 2},  // auto: 中图 -> 2
		{0, 4_000_000, 2},  // auto: 中图边界
		{0, 10_000_000, 1}, // auto: 超大图 -> 1
		{6, 10_000_000, 6}, // 显式优先，不被 auto 覆盖
		{99, 100, 6},       // 超上限钳到 6
	}
	for _, c := range cases {
		s := &Service{GridWebPMethod: c.configured}
		if got := s.gridWebPMethod(c.pixels); got != c.want {
			t.Errorf("gridWebPMethod(cfg=%d, px=%d)=%d, want %d", c.configured, c.pixels, got, c.want)
		}
	}
}
