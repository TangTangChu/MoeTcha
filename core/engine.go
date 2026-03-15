package core

import (
	"fmt"
	"math/rand"
	"time"
)

type Engine struct {
	idx *Indexer
	rng *rand.Rand
}

func NewEngine(idx *Indexer) *Engine {
	return &Engine{
		idx: idx,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type ChallengeType string

const (
	ChallengeGrid   ChallengeType = "grid"
	ChallengeClick  ChallengeType = "click"
	ChallengeRandom ChallengeType = "random"
)

type ChallengeInternal struct {
	Type     ChallengeType           `json:"type"`
	Question string                  `json:"question"`
	Tag      string                  `json:"tag"`
	Grid     *GridChallengeInternal  `json:"grid,omitempty"`
	Click    *ClickChallengeInternal `json:"click,omitempty"`
}

type GridChallengeInternal struct {
	Images          []GridItemInternal `json:"images"`
	CorrectImageIDs []string           `json:"correct_image_ids"`
}

type GridItemInternal struct {
	ImageID string `json:"image_id"`
	Path    string `json:"path"`
}

type ClickChallengeInternal struct {
	Image   ClickItemInternal `json:"image"`
	Regions []Region          `json:"regions"`
}

type ClickItemInternal struct {
	ImageID string `json:"image_id"`
	Path    string `json:"path"`
}

func (e *Engine) GenerateChallenge(kind ChallengeType) (*ChallengeInternal, error) {
	if kind == "" || kind == ChallengeRandom {
		if e.rng.Intn(2) == 0 {
			return e.GenerateGridChallenge()
		}
		return e.GenerateClickChallenge()
	}
	if kind == ChallengeGrid {
		return e.GenerateGridChallenge()
	}
	if kind == ChallengeClick {
		return e.GenerateClickChallenge()
	}
	return nil, fmt.Errorf("未知挑战类型: %s", kind)
}

func (e *Engine) GenerateGridChallenge() (*ChallengeInternal, error) {
	tags := e.idx.GetAllGridTags()
	if len(tags) == 0 {
		return nil, fmt.Errorf("没有可用的 Grid 标签")
	}

	tag := tags[e.rng.Intn(len(tags))]

	correctCandidates := e.idx.GetGridImagesByTag(tag)
	if len(correctCandidates) == 0 {
		return nil, fmt.Errorf("Grid 标签 %s 没有候选图片", tag)
	}

	correctCount := 2 + e.rng.Intn(3)
	if correctCount > len(correctCandidates) {
		correctCount = len(correctCandidates)
	}
	if correctCount > 9 {
		correctCount = 9
	}

	correct := pickUniqueGrid(e.rng, correctCandidates, correctCount)

	allGrid := e.idx.AllGridImages()
	if len(allGrid) < 9 {
		return nil, fmt.Errorf("Grid 图片总数不足 9 张")
	}

	distractorCount := 9 - correctCount
	distractors := make([]GridImageMeta, 0, distractorCount)
	for _, img := range shuffleGrid(e.rng, allGrid) {
		if hasTag(img.Tags, tag) {
			continue
		}
		distractors = append(distractors, img)
		if len(distractors) == distractorCount {
			break
		}
	}
	if len(distractors) != distractorCount {
		return nil, fmt.Errorf("无法凑齐干扰项，tag=%s", tag)
	}

	final := make([]GridImageMeta, 0, 9)
	final = append(final, correct...)
	final = append(final, distractors...)
	final = shuffleGrid(e.rng, final)

	items := make([]GridItemInternal, 0, len(final))
	for _, img := range final {
		items = append(items, GridItemInternal{
			ImageID: img.PackID + ":" + img.ID,
			Path:    img.Path,
		})
	}

	correctIDs := make([]string, 0, len(correct))
	for _, img := range correct {
		correctIDs = append(correctIDs, img.PackID+":"+img.ID)
	}

	return &ChallengeInternal{
		Type:     ChallengeGrid,
		Question: fmt.Sprintf("请选择所有包含【%s】的图片", tag),
		Tag:      tag,
		Grid: &GridChallengeInternal{
			Images:          items,
			CorrectImageIDs: correctIDs,
		},
	}, nil
}

func (e *Engine) GenerateClickChallenge() (*ChallengeInternal, error) {
	tags := e.idx.GetAllClickTags()
	if len(tags) == 0 {
		return nil, fmt.Errorf("没有可用的 Click 标签")
	}

	tag := tags[e.rng.Intn(len(tags))]

	candidates := e.idx.GetClickImagesByTag(tag)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("Click 标签 %s 没有候选图片", tag)
	}

	img := candidates[e.rng.Intn(len(candidates))]

	var regions []Region
	for _, r := range img.Regions {
		if r.Tag == tag {
			regions = append(regions, r)
		}
	}
	if len(regions) == 0 {
		return nil, fmt.Errorf("Click 图片中不存在目标 tag 的区域，tag=%s image=%s:%s", tag, img.PackID, img.ID)
	}

	return &ChallengeInternal{
		Type:     ChallengeClick,
		Question: fmt.Sprintf("请点击图片中所有【%s】的位置", tag),
		Tag:      tag,
		Click: &ClickChallengeInternal{
			Image: ClickItemInternal{
				ImageID: img.PackID + ":" + img.ID,
				Path:    img.Path,
			},
			Regions: regions,
		},
	}, nil
}

func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

func shuffleGrid(rng *rand.Rand, in []GridImageMeta) []GridImageMeta {
	out := make([]GridImageMeta, len(in))
	copy(out, in)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func pickUniqueGrid(rng *rand.Rand, in []GridImageMeta, n int) []GridImageMeta {
	if n <= 0 {
		return nil
	}
	if n >= len(in) {
		return shuffleGrid(rng, in)
	}
	shuffled := shuffleGrid(rng, in)
	out := make([]GridImageMeta, n)
	copy(out, shuffled[:n])
	return out
}
