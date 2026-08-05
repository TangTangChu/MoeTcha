package core

import (
	"fmt"
	"strings"
	"testing"
)

type mockPackProvider struct {
	packs []Pack
}

func (m *mockPackProvider) LoadPacks() ([]Pack, error) {
	return m.packs, nil
}

func buildTestIndexer(t *testing.T) *Indexer {
	t.Helper()
	provider := &mockPackProvider{
		packs: []Pack{
			{
				ID:       "animals",
				PackName: "动物测试包",
				Author:   "test",
				Version:  "1.0",
				TagDefs: map[string]TagDef{
					"猫": {Name: "猫", Similar: []string{"大猫"}},
					"狗": {Name: "狗"},
					"鸟": {Name: "鸟"},
				},
				Grid: &GridConfig{
					Size:       9,
					CorrectMin: 2,
					CorrectMax: 4,
				},
				GridImages: []GridImageMeta{
					{ID: "cat_01", File: "cat_01.webp", Tags: []string{"猫"}, PackID: "animals", Path: "/tmp/cat_01.webp"},
					{ID: "cat_02", File: "cat_02.webp", Tags: []string{"猫"}, PackID: "animals", Path: "/tmp/cat_02.webp"},
					{ID: "cat_03", File: "cat_03.webp", Tags: []string{"猫"}, PackID: "animals", Path: "/tmp/cat_03.webp"},
					{ID: "cat_04", File: "cat_04.webp", Tags: []string{"猫"}, PackID: "animals", Path: "/tmp/cat_04.webp"},
					{ID: "dog_01", File: "dog_01.webp", Tags: []string{"狗"}, PackID: "animals", Path: "/tmp/dog_01.webp"},
					{ID: "dog_02", File: "dog_02.webp", Tags: []string{"狗"}, PackID: "animals", Path: "/tmp/dog_02.webp"},
					{ID: "dog_03", File: "dog_03.webp", Tags: []string{"狗"}, PackID: "animals", Path: "/tmp/dog_03.webp"},
					{ID: "dog_04", File: "dog_04.webp", Tags: []string{"狗"}, PackID: "animals", Path: "/tmp/dog_04.webp"},
					{ID: "dog_05", File: "dog_05.webp", Tags: []string{"狗"}, PackID: "animals", Path: "/tmp/dog_05.webp"},
					{ID: "bird_01", File: "bird_01.webp", Tags: []string{"鸟"}, PackID: "animals", Path: "/tmp/bird_01.webp"},
					{ID: "bird_02", File: "bird_02.webp", Tags: []string{"鸟"}, PackID: "animals", Path: "/tmp/bird_02.webp"},
				},
				ClickImages: []ClickImageMeta{
					{
						ID: "scene_01", File: "scene_01.webp", PackID: "animals", Path: "/tmp/scene_01.webp",
						Regions: []Region{
							{Tag: "猫", X: 10, Y: 20, Width: 50, Height: 50},
							{Tag: "猫", X: 100, Y: 30, Width: 40, Height: 40},
						},
					},
					{
						ID: "scene_02", File: "scene_02.webp", PackID: "animals", Path: "/tmp/scene_02.webp",
						Regions: []Region{
							{Tag: "狗", X: 5, Y: 5, Width: 60, Height: 60},
						},
					},
				},
			},
		},
	}
	idx, err := NewIndexer(provider)
	if err != nil {
		t.Fatalf("NewIndexer failed: %v", err)
	}
	return idx
}

// TestEngineSetIndexerReload 验证运行期热替换索引（serve 控制台 reload 的底层）：
// 换索引后新请求只能看到新素材，旧索引的标签全部不可见。
func TestEngineSetIndexerReload(t *testing.T) {
	engine := NewEngine(buildTestIndexer(t))

	chal, err := engine.GenerateGridChallenge(DiffEasy)
	if err != nil {
		t.Fatalf("初始生成失败: %v", err)
	}
	switch chal.Tag {
	case "猫", "狗", "鸟":
	default:
		t.Fatalf("初始标签 = %q, 应为 animals 素材中的标签", chal.Tag)
	}

	plants := make([]GridImageMeta, 0, 11)
	for i := 1; i <= 11; i++ {
		id := fmt.Sprintf("leaf_%02d", i)
		plants = append(plants, GridImageMeta{
			ID:     id,
			File:   id + ".webp",
			Tags:   []string{"植物"},
			PackID: "plants",
			Path:   "/tmp/" + id + ".webp",
		})
	}
	idx2, err := BuildIndexer([]Pack{{
		ID:         "plants",
		PackName:   "植物测试包",
		TagDefs:    map[string]TagDef{"植物": {Name: "植物"}},
		Grid:       &GridConfig{Size: 9, CorrectMin: 2, CorrectMax: 4},
		GridImages: plants,
	}})
	if err != nil {
		t.Fatalf("BuildIndexer: %v", err)
	}

	engine.SetIndexer(idx2)
	if engine.Indexer() != idx2 {
		t.Error("SetIndexer 后 Indexer() 应返回新索引")
	}
	if cts := idx2.Counts(); cts.Packs != 1 || cts.GridImages != 11 || cts.GridTags != 1 {
		t.Errorf("Counts = %+v, want Packs=1 GridImages=11 GridTags=1", cts)
	}

	chal2, err := engine.GenerateGridChallenge(DiffEasy)
	if err != nil {
		t.Fatalf("换索引后生成失败: %v", err)
	}
	if chal2.Tag != "植物" {
		t.Errorf("换索引后标签 = %q, want 植物", chal2.Tag)
	}
	for _, img := range chal2.Grid.Images {
		if !strings.HasPrefix(img.Path, "/tmp/leaf_") {
			t.Errorf("换索引后仍出现旧素材路径: %s", img.Path)
		}
	}
}

func TestGenerateGridChallenge(t *testing.T) {
	idx := buildTestIndexer(t)
	engine := NewEngine(idx)

	chal, err := engine.GenerateGridChallenge(DiffEasy)
	if err != nil {
		t.Fatalf("GenerateGridChallenge failed: %v", err)
	}

	if chal.Type != ChallengeGrid {
		t.Errorf("Type = %q, want \"grid\"", chal.Type)
	}
	if chal.Grid == nil {
		t.Fatal("Grid is nil")
	}
	if len(chal.Grid.Images) != 9 {
		t.Errorf("Images count = %d, want 9", len(chal.Grid.Images))
	}
	if len(chal.Grid.CorrectImageIDs) == 0 {
		t.Error("CorrectImageIDs is empty")
	}
	if len(chal.Grid.CorrectImageIDs) > 9 {
		t.Errorf("CorrectImageIDs count = %d, too many", len(chal.Grid.CorrectImageIDs))
	}
	if chal.Question == "" {
		t.Error("Question is empty")
	}
	if chal.Tag == "" {
		t.Error("Tag is empty")
	}
	// Question should contain the tag's display name
	display := idx.TagDisplay(chal.Tag)
	if !strings.Contains(chal.Question, display) {
		t.Errorf("Question %q does not contain display name %q", chal.Question, display)
	}

	// Verify correct IDs are in images
	imageIDs := make(map[string]bool)
	for _, img := range chal.Grid.Images {
		imageIDs[img.ImageID] = true
	}
	for _, cid := range chal.Grid.CorrectImageIDs {
		if !imageIDs[cid] {
			t.Errorf("correct image ID %q not found in images list", cid)
		}
	}

	// image_id 现为不透明令牌，需经 Path 反查源图元信息。
	pathToMeta := make(map[string]GridImageMeta, len(idx.AllGridImages()))
	for _, m := range idx.AllGridImages() {
		pathToMeta[m.Path] = m
	}
	idToPath := make(map[string]string, len(chal.Grid.Images))
	for _, im := range chal.Grid.Images {
		idToPath[im.ImageID] = im.Path
	}
	// All correct images should have the same tag
	for _, cid := range chal.Grid.CorrectImageIDs {
		p, ok := idToPath[cid]
		if !ok {
			t.Errorf("correct image ID %q not found in images list", cid)
			continue
		}
		img, ok := pathToMeta[p]
		if !ok {
			continue
		}
		if !hasTag(img.Tags, chal.Tag) {
			t.Errorf("correct image %q does not have tag %q", cid, chal.Tag)
		}
	}
}

func TestGenerateClickChallenge(t *testing.T) {
	idx := buildTestIndexer(t)
	engine := NewEngine(idx)

	chal, err := engine.GenerateClickChallenge(DiffMedium)
	if err != nil {
		t.Fatalf("GenerateClickChallenge failed: %v", err)
	}

	if chal.Type != ChallengeClick {
		t.Errorf("Type = %q, want \"click\"", chal.Type)
	}
	if chal.Click == nil {
		t.Fatal("Click is nil")
	}
	if chal.Click.Image.ImageID == "" {
		t.Error("ImageID is empty")
	}
	if chal.Click.Image.Path == "" {
		t.Error("Path is empty")
	}
	if len(chal.Click.Regions) == 0 {
		t.Error("Regions is empty")
	}
	if chal.Question == "" {
		t.Error("Question is empty")
	}
}

func TestGenerateRandomChallenge(t *testing.T) {
	idx := buildTestIndexer(t)
	engine := NewEngine(idx)

	chal, err := engine.GenerateChallenge(ChallengeRandom, DiffEasy)
	if err != nil {
		t.Fatalf("GenerateChallenge(random) failed: %v", err)
	}
	if chal == nil {
		t.Fatal("chal is nil")
	}
	if chal.Type != ChallengeGrid && chal.Type != ChallengeClick {
		t.Errorf("Type = %q, want grid or click", chal.Type)
	}
}

func TestGenerateChallengeExplicitType(t *testing.T) {
	idx := buildTestIndexer(t)
	engine := NewEngine(idx)

	chal, err := engine.GenerateChallenge(ChallengeGrid, DiffEasy)
	if err != nil {
		t.Fatalf("GenerateChallenge(grid) failed: %v", err)
	}
	if chal.Type != ChallengeGrid {
		t.Errorf("Type = %q, want grid", chal.Type)
	}

	chal, err = engine.GenerateChallenge(ChallengeClick, DiffEasy)
	if err != nil {
		t.Fatalf("GenerateChallenge(click) failed: %v", err)
	}
	if chal.Type != ChallengeClick {
		t.Errorf("Type = %q, want click", chal.Type)
	}
}

func TestGenerateChallengeInvalidType(t *testing.T) {
	idx := buildTestIndexer(t)
	engine := NewEngine(idx)

	_, err := engine.GenerateChallenge("invalid", DiffEasy)
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestDifficultyEasy(t *testing.T) {
	idx := buildTestIndexer(t)
	engine := NewEngine(idx)

	// Easy mode: should not pick from similar tags as distractors
	chal, err := engine.GenerateGridChallenge(DiffEasy)
	if err != nil {
		t.Fatalf("GenerateGridChallenge(easy) failed: %v", err)
	}

	// The only similar tag is "大猫" which has no images, so easy mode trivially
	// works. Verify the distractors don't include the target tag images.
	correctSet := make(map[string]bool)
	for _, cid := range chal.Grid.CorrectImageIDs {
		correctSet[cid] = true
	}
	pathToMeta := make(map[string]GridImageMeta, len(idx.AllGridImages()))
	for _, m := range idx.AllGridImages() {
		pathToMeta[m.Path] = m
	}
	for _, img := range chal.Grid.Images {
		if correctSet[img.ImageID] {
			continue
		}
		meta, ok := pathToMeta[img.Path]
		if !ok {
			continue
		}
		if hasTag(meta.Tags, chal.Tag) {
			t.Errorf("distractor %q has target tag %q", img.Path, chal.Tag)
		}
	}
}

func TestTagDisplay(t *testing.T) {
	idx := buildTestIndexer(t)

	if got := idx.TagDisplay("猫"); got != "猫" {
		t.Errorf("TagDisplay(猫) = %q, want 猫", got)
	}
	if got := idx.TagDisplay("unknown"); got != "unknown" {
		t.Errorf("TagDisplay(unknown) = %q, want unknown", got)
	}
}

func TestSimilarTags(t *testing.T) {
	idx := buildTestIndexer(t)

	similar := idx.SimilarTags("猫")
	if len(similar) != 1 || similar[0] != "大猫" {
		t.Errorf("SimilarTags(猫) = %v, want [大猫]", similar)
	}
	if s := idx.SimilarTags("狗"); len(s) != 0 {
		t.Errorf("SimilarTags(狗) should be empty, got %v", s)
	}
}

func TestGridConfigForTag(t *testing.T) {
	idx := buildTestIndexer(t)

	cfg := idx.GridConfigForTag("猫")
	if cfg.Size != 9 {
		t.Errorf("GridConfigForTag size = %d, want 9", cfg.Size)
	}
}

func TestClickConfigForTag(t *testing.T) {
	idx := buildTestIndexer(t)

	cfg := idx.ClickConfigForTag("猫")
	if cfg.Question != defaultClickConfig().Question {
		t.Errorf("ClickConfigForTag question = %q, want default", cfg.Question)
	}
}

func buildClickCountIndexer(t *testing.T, count int) *Indexer {
	t.Helper()
	provider := &mockPackProvider{
		packs: []Pack{
			{
				ID:       "scenes",
				PackName: "场景测试包",
				TagDefs:  map[string]TagDef{"猫": {Name: "猫"}},
				Click: &ClickConfig{
					Count: count,
				},
				ClickImages: []ClickImageMeta{
					{
						ID: "scene_01", File: "scene_01.webp", PackID: "scenes", Path: "/tmp/scene_01.webp",
						Regions: []Region{
							{Tag: "猫", X: 10, Y: 10, Width: 50, Height: 50},
							{Tag: "猫", X: 100, Y: 20, Width: 40, Height: 40},
						},
					},
				},
			},
		},
	}
	idx, err := NewIndexer(provider)
	if err != nil {
		t.Fatalf("NewIndexer failed: %v", err)
	}
	return idx
}

func TestGenerateClickChallengeCount(t *testing.T) {
	cases := []struct {
		name        string
		count       int
		wantReq     int
		wantContain string
	}{
		{"count=1 取 1", 1, 1, "1个"},
		{"count=2 取全部正好", 2, 2, "2个"},
		{"count=0 退化为全部", 0, 2, "所有"},
		{"count=5 超过可用退化为全部", 5, 2, "所有"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idx := buildClickCountIndexer(t, c.count)
			engine := NewEngine(idx)
			chal, err := engine.GenerateClickChallenge(DiffMedium)
			if err != nil {
				t.Fatalf("GenerateClickChallenge failed: %v", err)
			}
			if chal.Click == nil || chal.Click.Required != c.wantReq {
				got := 0
				if chal.Click != nil {
					got = chal.Click.Required
				}
				t.Errorf("Required = %d, want %d", got, c.wantReq)
			}
			if !strings.Contains(chal.Question, c.wantContain) {
				t.Errorf("Question = %q, want substring %q", chal.Question, c.wantContain)
			}
		})
	}
}

// TestGenerateClickChallengeDifficultyEasy 验证 pack 未指定 Count（默认“点击全部”）时，
// DiffEasy 把 required 减半并在题面揭示数量，DiffMedium 维持“点击全部”。
func TestGenerateClickChallengeDifficultyEasy(t *testing.T) {
	cases := []struct {
		name        string
		diff        Difficulty
		wantReq     int
		wantContain string
	}{
		{"easy 减半并揭示数量", DiffEasy, 1, "1个"},
		{"medium 维持全部", DiffMedium, 2, "所有"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// buildClickCountIndexer(t, 0) 提供一张含 2 个「猫」区域、Count=0 的图。
			idx := buildClickCountIndexer(t, 0)
			engine := NewEngine(idx)
			chal, err := engine.GenerateClickChallenge(c.diff)
			if err != nil {
				t.Fatalf("GenerateClickChallenge failed: %v", err)
			}
			if chal.Click == nil || chal.Click.Required != c.wantReq {
				got := 0
				if chal.Click != nil {
					got = chal.Click.Required
				}
				t.Errorf("Required = %d, want %d", got, c.wantReq)
			}
			if !strings.Contains(chal.Question, c.wantContain) {
				t.Errorf("Question = %q, want substring %q", chal.Question, c.wantContain)
			}
		})
	}
}
