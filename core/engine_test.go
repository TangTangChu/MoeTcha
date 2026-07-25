package core

import (
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
				ID:          "animals",
				PackName:    "动物测试包",
				Author:      "test",
				Version:     "1.0",
				Description: "for testing",
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

func TestGenerateGridChallenge(t *testing.T) {
	idx := buildTestIndexer(t)
	engine := NewEngine(idx)

	chal, err := engine.GenerateGridChallenge()
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

	// All correct images should have the same tag
	for _, cid := range chal.Grid.CorrectImageIDs {
		img, ok := idx.GetGridImage(cid)
		if !ok {
			t.Errorf("image ID %q not in indexer", cid)
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

	chal, err := engine.GenerateClickChallenge()
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

	chal, err := engine.GenerateChallenge(ChallengeRandom)
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

	chal, err := engine.GenerateChallenge(ChallengeGrid)
	if err != nil {
		t.Fatalf("GenerateChallenge(grid) failed: %v", err)
	}
	if chal.Type != ChallengeGrid {
		t.Errorf("Type = %q, want grid", chal.Type)
	}

	chal, err = engine.GenerateChallenge(ChallengeClick)
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

	_, err := engine.GenerateChallenge("invalid")
	if err == nil {
		t.Error("expected error for invalid type")
	}
}
