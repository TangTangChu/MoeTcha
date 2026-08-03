package core

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"
)

type Engine struct {
	// idx 用原子指针承载：控制台 reload 命令会整体替换索引，在途请求仍持有
	// 旧索引（它们已 Load 出指针），新请求立即看到新素材，无需加锁。
	idx atomic.Pointer[Indexer]
}

func NewEngine(idx *Indexer) *Engine {
	e := &Engine{}
	e.idx.Store(idx)
	return e
}

// SetIndexer 运行期整体替换索引（serve 控制台的 reload 命令）。
func (e *Engine) SetIndexer(idx *Indexer) {
	e.idx.Store(idx)
}

// Indexer 返回当前生效的索引。
func (e *Engine) Indexer() *Indexer {
	return e.idx.Load()
}

// newRNG 构造一个请求级 *rand.Rand，混入 crypto 熵避免高并发下纳秒种子碰撞。
func (e *Engine) newRNG() *rand.Rand {
	var b [8]byte
	_, _ = crand.Read(b[:])
	seed := int64(binary.BigEndian.Uint64(b[:]))
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return rand.New(rand.NewSource(seed))
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
	Image    ClickItemInternal `json:"image"`
	Regions  []Region          `json:"regions"`
	Required int               `json:"required,omitempty"` // 需点击的目标数量；0 视为全部（兼容旧数据）
}

type ClickItemInternal struct {
	ImageID string `json:"image_id"`
	Path    string `json:"path"`
}

// --- 公开入口 ---

func (e *Engine) GenerateChallenge(kind ChallengeType, diff Difficulty) (*ChallengeInternal, error) {
	rng := e.newRNG()
	if kind == "" || kind == ChallengeRandom {
		if rng.Intn(2) == 0 {
			return e.GenerateGridChallenge(diff)
		}
		return e.GenerateClickChallenge(diff)
	}
	if kind == ChallengeGrid {
		return e.GenerateGridChallenge(diff)
	}
	if kind == ChallengeClick {
		return e.GenerateClickChallenge(diff)
	}
	return nil, fmt.Errorf("未知挑战类型: %s", kind)
}

// --- Grid ---

func (e *Engine) GenerateGridChallenge(diff Difficulty) (*ChallengeInternal, error) {
	rng := e.newRNG()
	tags := e.idx.Load().GetAllGridTags()
	if len(tags) == 0 {
		return nil, fmt.Errorf("没有可用的 Grid 标签")
	}

	tag := tags[rng.Intn(len(tags))]
	cfg := e.idx.Load().GridConfigForTag(tag)

	correctCandidates := e.idx.Load().GetGridImagesByTag(tag)
	if len(correctCandidates) == 0 {
		return nil, fmt.Errorf("Grid 标签 %s 没有候选图片", tag)
	}

	correctCount := cfg.correctPickCount(rng)
	if correctCount > len(correctCandidates) {
		correctCount = len(correctCandidates)
	}
	if correctCount < 1 {
		correctCount = 1
	}
	// correctCount 不得超过 cfg.Size，否则 distGoal=Size-correctCount 为负，
	// 会让后续 distractors[:distGoal] 越界 panic（CorrectMax > Size 的错配场景）。
	if cfg.Size > 0 && correctCount > cfg.Size {
		correctCount = cfg.Size
	}

	correct := pickUniqueGrid(rng, correctCandidates, correctCount)
	allGrid := e.idx.Load().AllGridImages()

	distGoal := cfg.Size - correctCount
	distractors := e.pickDistractorsWithRNG(rng, tag, distGoal, diff, allGrid)

	// 干扰图不足时，尝试增加正确图片数量补齐到 cfg.Size（在 correctMax 与候选数允许范围内）。
	// 注意方向：干扰不足时应 *增加* correctCount（让正确图填空），而不是减少——
	// 减少 correctCount 会让 distGoal 更大、缺口更深，反而得到更小的网格（历史上的 bug）。
	correctLo := cfg.CorrectMin
	if correctLo <= 0 {
		correctLo = 1
	}
	correctHi := cfg.CorrectMax
	if correctHi < correctLo {
		correctHi = correctLo
	}
	maxCorrect := correctHi
	if maxCorrect > len(correctCandidates) {
		maxCorrect = len(correctCandidates)
	}
	if cfg.Size > 0 && maxCorrect > cfg.Size {
		maxCorrect = cfg.Size
	}
	for len(distractors) < distGoal && correctCount < maxCorrect {
		correctCount++
		distGoal = cfg.Size - correctCount
		correct = pickUniqueGrid(rng, correctCandidates, correctCount)
		distractors = e.pickDistractorsWithRNG(rng, tag, distGoal, diff, allGrid)
	}
	if len(distractors) > distGoal {
		distractors = distractors[:distGoal]
	}
	actualDist := len(distractors)

	total := correctCount + actualDist
	if total < 3 {
		return nil, fmt.Errorf("Grid 可用图片不足（至少需要 3 张），tag=%s total=%d", tag, total)
	}

	final := make([]GridImageMeta, 0, total)
	final = append(final, correct...)
	final = append(final, distractors...)
	final = shuffleGrid(rng, final)

	// 为每张图片分配一个本次挑战内不透明的随机 ID，替代 "PackID:文件名"，
	// 避免向客户端泄露源文件名与素材包目录结构。同一张源图在本次挑战内
	// 映射到同一 opaque ID，使 /verify 的集合判等仍然成立。
	opaque := make(map[string]string, len(final))
	usedID := make(map[string]struct{}, len(final))
	newOpaqueID := func() string {
		for {
			id := RandomHex(8)
			if _, dup := usedID[id]; !dup {
				usedID[id] = struct{}{}
				return id
			}
		}
	}
	for _, img := range final {
		realID := img.PackID + ":" + img.ID
		if _, ok := opaque[realID]; !ok {
			opaque[realID] = newOpaqueID()
		}
	}

	items := make([]GridItemInternal, 0, len(final))
	for _, img := range final {
		items = append(items, GridItemInternal{
			ImageID: opaque[img.PackID+":"+img.ID],
			Path:    img.Path,
		})
	}

	correctIDs := make([]string, 0, len(correct))
	for _, img := range correct {
		correctIDs = append(correctIDs, opaque[img.PackID+":"+img.ID])
	}

	question := buildQuestion(cfg.Question, e.idx.Load().TagDisplay(tag))

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
	similarTags := e.idx.Load().SimilarTags(tag)
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

func hasAnyTag(tags []string, targetSet map[string]struct{}) bool {
	for _, t := range tags {
		if _, ok := targetSet[t]; ok {
			return true
		}
	}
	return false
}

// --- Click ---

func (e *Engine) GenerateClickChallenge(diff Difficulty) (*ChallengeInternal, error) {
	rng := e.newRNG()
	tags := e.idx.Load().GetAllClickTags()
	if len(tags) == 0 {
		return nil, fmt.Errorf("没有可用的 Click 标签")
	}

	tag := tags[rng.Intn(len(tags))]

	candidates := e.idx.Load().GetClickImagesByTag(tag)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("Click 标签 %s 没有候选图片", tag)
	}

	// matchingRegions 返回图片中标签匹配的区域。
	matchingRegions := func(img ClickImageMeta) []Region {
		out := make([]Region, 0, len(img.Regions))
		for _, r := range img.Regions {
			if r.Tag == tag {
				out = append(out, r)
			}
		}
		return out
	}

	cfg := e.idx.Load().ClickConfigForTag(tag)

	// 配置了 Count 时优先选择匹配区域数 >= Count 的图片，避免题目数量与可见区域不符。
	pool := candidates
	if cfg.Count > 0 {
		filtered := make([]ClickImageMeta, 0, len(candidates))
		for _, c := range candidates {
			if len(matchingRegions(c)) >= cfg.Count {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	}

	img := pool[rng.Intn(len(pool))]

	regions := matchingRegions(img)
	if len(regions) == 0 {
		return nil, fmt.Errorf("Click 图片中不存在目标 tag 的区域，tag=%s image=%s:%s", tag, img.PackID, img.ID)
	}

	// 解析需点击数量：Count 在 (0, len(regions)] 时取 Count，否则退化为点击全部。
	var required int
	var countForQuestion int
	switch {
	case cfg.Count > 0 && cfg.Count <= len(regions):
		required = cfg.Count
		countForQuestion = cfg.Count
	default:
		required = len(regions)
		countForQuestion = 0 // 0 -> "所有"
	}

	// 难度调节：仅在 pack 未显式指定 Count（默认“点击全部”）时生效。
	// easy 减少需点击数量（识别负担更轻）并在题面揭示具体数量，让用户知道何时停；
	// medium/hard 维持“点击全部”。pack 显式指定 Count 时尊重作者意图，不被难度覆盖。
	// 注：click 的难度维度较 grid 弱（无干扰图相似度可调），hard 与 medium 均为“点全部”。
	if cfg.Count <= 0 && diff == DiffEasy && required > 1 {
		required = (required + 1) / 2 // ceil(half)
		countForQuestion = required   // 揭示具体数量
	}

	question := buildClickQuestion(cfg.Question, e.idx.Load().TagDisplay(tag), countForQuestion)

	return &ChallengeInternal{
		Type:     ChallengeClick,
		Question: question,
		Tag:      tag,
		Click: &ClickChallengeInternal{
			Image: ClickItemInternal{
				// 不透明随机 ID：click 仅按点击坐标判分，image_id 不参与验证，
				// 此处赋随机值仅为响应字段完整，同时避免泄露源文件名与素材包。
				ImageID: RandomHex(8),
				Path:    img.Path,
			},
			Regions:  regions,
			Required: required,
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

// buildClickQuestion 构造点击挑战的问题文案。
// {count} 占位符：count<=0 替换为「所有」，否则替换为「N 个」。
func buildClickQuestion(tmpl, display string, count int) string {
	if tmpl == "" {
		tmpl = "请点击图中{count}「{tag}」"
	}
	countStr := "所有"
	if count > 0 {
		countStr = fmt.Sprintf("%d个", count)
	}
	out := strings.ReplaceAll(tmpl, "{count}", countStr)
	return strings.ReplaceAll(out, "{tag}", display)
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
