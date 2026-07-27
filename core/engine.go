package core

import (
	"fmt"
	"math/rand"
	"strings"
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

// --- 公开入口 ---

func (e *Engine) GenerateChallenge(kind ChallengeType, diff Difficulty) (*ChallengeInternal, error) {
	if kind == "" || kind == ChallengeRandom {
		if e.rng.Intn(2) == 0 {
			return e.GenerateGridChallenge(diff)
		}
		return e.GenerateClickChallenge()
	}
	if kind == ChallengeGrid {
		return e.GenerateGridChallenge(diff)
	}
	if kind == ChallengeClick {
		return e.GenerateClickChallenge()
	}
	return nil, fmt.Errorf("未知挑战类型: %s", kind)
}

// --- Grid ---

func (e *Engine) GenerateGridChallenge(diff Difficulty) (*ChallengeInternal, error) {
	tags := e.idx.GetAllGridTags()
	if len(tags) == 0 {
		return nil, fmt.Errorf("没有可用的 Grid 标签")
	}

	tag := tags[e.rng.Intn(len(tags))]
	cfg := e.idx.GridConfigForTag(tag)

	correctCandidates := e.idx.GetGridImagesByTag(tag)
	if len(correctCandidates) == 0 {
		return nil, fmt.Errorf("Grid 标签 %s 没有候选图片", tag)
	}

	// 确定正确数（不超过候选数）
	correctCount := cfg.correctPickCount(e.rng)
	if correctCount > len(correctCandidates) {
		correctCount = len(correctCandidates)
	}
	if correctCount < 1 {
		correctCount = 1
	}

	correct := pickUniqueGrid(e.rng, correctCandidates, correctCount)
	allGrid := e.idx.AllGridImages()

	// 干扰项选取 + 降级
	distGoal := cfg.Size - correctCount
	distractors := e.pickDistractorsWithRNG(e.rng, tag, distGoal, diff, allGrid)

	// 不够 → 减少正确数以降低干扰项需求
	for correctCount > 1 && len(distractors) < cfg.Size-correctCount {
		correctCount--
		correct = pickUniqueGrid(e.rng, correctCandidates, correctCount)
		distGoal = cfg.Size - correctCount
		distractors = e.pickDistractorsWithRNG(e.rng, tag, distGoal, diff, allGrid)
	}
	// 还是不够 → 有多少拿多少，缩小网格
	if len(distractors) > distGoal {
		distractors = distractors[:distGoal]
	}
	actualDist := len(distractors)

	total := correctCount + actualDist
	if total < 3 {
		return nil, fmt.Errorf("Grid 可用图片不足（至少需要 3 张），tag=%s total=%d", tag, total)
	}

	// 合并 + 打乱
	final := make([]GridImageMeta, 0, total)
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

	question := buildQuestion(cfg.Question, e.idx.TagDisplay(tag))

	return &ChallengeInternal{
		Type:     ChallengeGrid,
		Question: question,
		Tag:      tag,
		Grid: &GridChallengeInternal{
			Images:          items,
			CorrectImageIDs: correctIDs,
		},
	}, nil
}

// correctPickCount 在 [min, max] 内随机。
func (cfg GridConfig) correctPickCount(rng *rand.Rand) int {
	lo, hi := cfg.CorrectMin, cfg.CorrectMax
	if lo <= 0 {
		lo = 1
	}
	if hi < lo {
		hi = lo
	}
	if lo == hi {
		return lo
	}
	return lo + rng.Intn(hi-lo+1)
}

// pickDistractorsWithRNG 按难度从全部 grid 图片中挑选干扰项。
func (e *Engine) pickDistractorsWithRNG(rng *rand.Rand, tag string, need int, diff Difficulty, all []GridImageMeta) []GridImageMeta {
	similarTags := e.idx.SimilarTags(tag)
	similarSet := make(map[string]struct{}, len(similarTags))
	for _, t := range similarTags {
		similarSet[t] = struct{}{}
	}

	var similarPool, regularPool []GridImageMeta
	for _, img := range all {
		if hasTag(img.Tags, tag) {
			continue
		}
		if hasAnyTag(img.Tags, similarSet) {
			similarPool = append(similarPool, img)
		} else {
			regularPool = append(regularPool, img)
		}
	}

	similarPool = shuffleGrid(rng, similarPool)
	regularPool = shuffleGrid(rng, regularPool)

	var out []GridImageMeta
	similarUsed, regularUsed := 0, 0
	appendPool := func(pool []GridImageMeta, used *int, n int) {
		if n <= 0 || *used >= len(pool) {
			return
		}
		available := pool[*used:]
		if n > len(available) {
			n = len(available)
		}
		out = append(out, available[:n]...)
		*used += n
	}

	switch diff {
	case DiffEasy:
		// 只用不相似的干扰项
		appendPool(regularPool, &regularUsed, need)
	case DiffMedium:
		// 一半相似一半不相似，缺一侧时从另一侧补齐。
		half := need / 2
		appendPool(similarPool, &similarUsed, half)
		appendPool(regularPool, &regularUsed, need-len(out))
	case DiffHard:
		// 尽量用相似的
		appendPool(similarPool, &similarUsed, need)
		appendPool(regularPool, &regularUsed, need-len(out))
	default:
		appendPool(regularPool, &regularUsed, need)
	}

	// 仍不够则从尚未使用的另一类补齐。
	if len(out) < need {
		appendPool(similarPool, &similarUsed, need-len(out))
	}
	if len(out) < need {
		appendPool(regularPool, &regularUsed, need-len(out))
	}

	return out
}

func takeN(in []GridImageMeta, n int) []GridImageMeta {
	if n <= 0 || len(in) == 0 {
		return nil
	}
	if n > len(in) {
		n = len(in)
	}
	return in[:n]
}

func hasAnyTag(tags []string, targetSet map[string]struct{}) bool {
	for _, t := range tags {
		if _, ok := targetSet[t]; ok {
			return true
		}
	}
	return false
}

// --- Click ---

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

	cfg := e.idx.ClickConfigForTag(tag)
	question := buildQuestion(cfg.Question, e.idx.TagDisplay(tag))

	return &ChallengeInternal{
		Type:     ChallengeClick,
		Question: question,
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

// --- 工具函数 ---

func buildQuestion(tmpl, display string) string {
	if tmpl == "" {
		tmpl = "请选出所有「{tag}」"
	}
	return strings.ReplaceAll(tmpl, "{tag}", display)
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
