package core

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	crand "crypto/rand"
	"encoding/binary"

	"moetcha/core/render"
)

const (
	defaultGeneratedTileSize = 160
	maxGeneratedImageCount   = 100
	maxGeneratedDimension    = 16384
	maxGeneratedPixels       = 64_000_000
)

type GridImageGenerateRequest struct {
	Tag                 string     `json:"tag"`
	ImageCount          int        `json:"image_count"`
	Size                int        `json:"size"`
	CorrectCount        int        `json:"correct_count"`
	ImageIDs            []string   `json:"image_ids"`
	CorrectImageIDs     []string   `json:"correct_image_ids"`
	CorrectNumbers      []int      `json:"correct_numbers"`
	DistractorImageIDs  []string   `json:"distractor_image_ids"`
	Difficulty          Difficulty `json:"difficulty"`
	Rows                int        `json:"rows"`
	Columns             int        `json:"columns"`
	Cols                int        `json:"cols"`
	TileWidth           int        `json:"tile_width"`
	TileHeight          int        `json:"tile_height"`
	Gap                 int        `json:"gap"`
	Padding             int        `json:"padding"`
	Fit                 string     `json:"fit"`
	Background          string     `json:"background"`
	ShowLabels          *bool      `json:"show_labels"`
	LabelScale          int        `json:"label_scale"`
	LabelPosition       string     `json:"label_position"`
	LabelColor          string     `json:"label_color"`
	LabelBackground     string     `json:"label_background"`
	Quality             int        `json:"quality"`
	Shuffle             *bool      `json:"shuffle"`
	Seed                *int64     `json:"seed"`
	ApplyRenderer       *bool      `json:"apply_renderer"`
	TemporaryTTLSeconds int        `json:"temporary_ttl_seconds"`
}

type GridImageGeneratedTile struct {
	Number  int      `json:"number"`
	ImageID string   `json:"image_id"`
	File    string   `json:"file"`
	Tags    []string `json:"tags"`
	Correct bool     `json:"correct"`
	X       int      `json:"x"`
	Y       int      `json:"y"`
	Width   int      `json:"width"`
	Height  int      `json:"height"`
}

type GridImageGenerateResult struct {
	ID               string                   `json:"id"`
	AssetKey         string                   `json:"asset_key"`
	AssetURL         string                   `json:"asset_url"`
	TemporaryFileURL string                   `json:"temporary_file_url"`
	ContentType      string                   `json:"content_type"`
	CreatedAt        time.Time                `json:"created_at"`
	ExpiresAt        time.Time                `json:"expires_at"`
	Tag              string                   `json:"tag"`
	TagDisplay       string                   `json:"tag_display"`
	Question         string                   `json:"question"`
	Difficulty       Difficulty               `json:"difficulty"`
	Seed             int64                    `json:"seed"`
	ImageCount       int                      `json:"image_count"`
	CorrectCount     int                      `json:"correct_count"`
	CorrectNumbers   []int                    `json:"correct_numbers"`
	CorrectImageIDs  []string                 `json:"correct_image_ids"`
	Width            int                      `json:"width"`
	Height           int                      `json:"height"`
	Rows             int                      `json:"rows"`
	Columns          int                      `json:"columns"`
	TileWidth        int                      `json:"tile_width"`
	TileHeight       int                      `json:"tile_height"`
	Gap              int                      `json:"gap"`
	Padding          int                      `json:"padding"`
	Fit              string                   `json:"fit"`
	Background       string                   `json:"background"`
	Quality          int                      `json:"quality"`
	Shuffle          bool                     `json:"shuffle"`
	ShowLabels       bool                     `json:"show_labels"`
	LabelScale       int                      `json:"label_scale"`
	LabelPosition    string                   `json:"label_position"`
	LabelColor       string                   `json:"label_color"`
	LabelBackground  string                   `json:"label_background"`
	ApplyRenderer    bool                     `json:"apply_renderer"`
	Tiles            []GridImageGeneratedTile `json:"tiles"`
}

type GridImageRequestError struct {
	message string
}

func (e *GridImageRequestError) Error() string {
	return e.message
}

func IsGridImageRequestError(err error) bool {
	var target *GridImageRequestError
	return errors.As(err, &target)
}

func gridImageRequestError(format string, args ...any) error {
	return &GridImageRequestError{message: fmt.Sprintf(format, args...)}
}

type gridImagePlanTile struct {
	meta    GridImageMeta
	correct bool
}

type gridImagePlan struct {
	tag                 string
	tagDisplay          string
	question            string
	difficulty          Difficulty
	seed                int64
	tiles               []gridImagePlanTile
	correctNumbers      []int
	correctImageIDs     []string
	compose             render.GridComposeOptions
	quality             int
	shuffle             bool
	applyRenderer       bool
	temporaryTTLSeconds int
}

func (e *Engine) buildGridImagePlan(req GridImageGenerateRequest, defaultDifficulty Difficulty) (*gridImagePlan, error) {
	if e == nil || e.idx == nil {
		return nil, fmt.Errorf("grid engine 未初始化")
	}
	if req.ImageCount < 0 || req.Size < 0 || req.CorrectCount < 0 {
		return nil, gridImageRequestError("image_count、size、correct_count 不能为负数")
	}
	if len(req.ImageIDs) > maxGeneratedImageCount ||
		len(req.CorrectImageIDs) > maxGeneratedImageCount ||
		len(req.DistractorImageIDs) > maxGeneratedImageCount ||
		len(req.CorrectNumbers) > maxGeneratedImageCount {
		return nil, gridImageRequestError("图片列表长度不能超过 %d", maxGeneratedImageCount)
	}
	if len(req.ImageIDs) > 0 && len(req.DistractorImageIDs) > 0 {
		return nil, gridImageRequestError("image_ids 与 distractor_image_ids 不能同时使用")
	}

	seed := mixSeed(req.Seed)
	rng := rand.New(rand.NewSource(seed))

	explicitImages, err := e.resolveGridImageIDs(req.ImageIDs)
	if err != nil {
		return nil, err
	}
	explicitCorrect, err := e.resolveGridImageIDs(req.CorrectImageIDs)
	if err != nil {
		return nil, err
	}
	explicitDistractors, err := e.resolveGridImageIDs(req.DistractorImageIDs)
	if err != nil {
		return nil, err
	}
	if len(req.CorrectNumbers) > 0 {
		if len(explicitImages) == 0 {
			return nil, gridImageRequestError("correct_numbers 只能与 image_ids 一起使用")
		}
		numberCorrect, err := imagesAtNumbers(explicitImages, req.CorrectNumbers)
		if err != nil {
			return nil, err
		}
		if len(explicitCorrect) > 0 && !sameGridImageSet(explicitCorrect, numberCorrect) {
			return nil, gridImageRequestError("correct_numbers 与 correct_image_ids 指向的图片不一致")
		}
		explicitCorrect = numberCorrect
	}

	tag := strings.TrimSpace(req.Tag)
	if tag == "" && len(explicitCorrect) > 0 {
		tag, err = commonGridTag(explicitCorrect)
		if err != nil {
			return nil, err
		}
	}
	if tag == "" {
		if len(explicitImages) > 0 {
			return nil, gridImageRequestError("使用 image_ids 时必须提供 tag、correct_image_ids 或 correct_numbers")
		}
		tags := e.idx.GetAllGridTags()
		if len(tags) == 0 {
			return nil, gridImageRequestError("没有可用的 Grid 标签")
		}
		tag = tags[rng.Intn(len(tags))]
	}

	cfg := e.idx.GridConfigForTag(tag)
	imageCount, err := resolveRequestedImageCount(req, cfg.Size, len(explicitImages))
	if err != nil {
		return nil, err
	}
	if imageCount < 1 || imageCount > maxGeneratedImageCount {
		return nil, gridImageRequestError("image_count 必须在 1~%d，当前=%d", maxGeneratedImageCount, imageCount)
	}

	difficulty := req.Difficulty
	if difficulty == "" {
		difficulty = defaultDifficulty
	}
	if difficulty == "" {
		difficulty = DiffEasy
	}
	if difficulty != DiffEasy && difficulty != DiffMedium && difficulty != DiffHard {
		return nil, gridImageRequestError("difficulty 必须为 easy / medium / hard")
	}

	shuffle := true
	if req.Shuffle != nil {
		shuffle = *req.Shuffle
	}

	var chosen []GridImageMeta
	var correctSet map[string]struct{}
	if len(explicitImages) > 0 {
		chosen = explicitImages
		correctSet, err = validateExplicitGridImages(tag, chosen, explicitCorrect, req.CorrectCount)
		if err != nil {
			return nil, err
		}
	} else {
		chosen, correctSet, err = e.selectGeneratedGridImages(
			rng,
			tag,
			imageCount,
			req.CorrectCount,
			cfg,
			difficulty,
			explicitCorrect,
			explicitDistractors,
		)
		if err != nil {
			return nil, err
		}
	}

	if shuffle {
		chosen = shuffleGrid(rng, chosen)
	}

	rows, columns, err := resolveGridLayout(imageCount, req.Rows, req.Columns, req.Cols)
	if err != nil {
		return nil, err
	}
	compose, quality, applyRenderer, ttlSeconds, err := normalizeGridRenderOptions(req, rows, columns)
	if err != nil {
		return nil, err
	}

	tiles := make([]gridImagePlanTile, 0, len(chosen))
	correctNumbers := make([]int, 0, len(correctSet))
	correctImageIDs := make([]string, 0, len(correctSet))
	for i, img := range chosen {
		id := globalGridImageID(img)
		_, correct := correctSet[id]
		tiles = append(tiles, gridImagePlanTile{meta: img, correct: correct})
		if correct {
			correctNumbers = append(correctNumbers, i+1)
			correctImageIDs = append(correctImageIDs, id)
		}
	}

	return &gridImagePlan{
		tag:                 tag,
		tagDisplay:          e.idx.TagDisplay(tag),
		question:            buildQuestion(cfg.Question, e.idx.TagDisplay(tag)),
		difficulty:          difficulty,
		seed:                seed,
		tiles:               tiles,
		correctNumbers:      correctNumbers,
		correctImageIDs:     correctImageIDs,
		compose:             compose,
		quality:             quality,
		shuffle:             shuffle,
		applyRenderer:       applyRenderer,
		temporaryTTLSeconds: ttlSeconds,
	}, nil
}

func (e *Engine) selectGeneratedGridImages(
	rng *rand.Rand,
	tag string,
	imageCount int,
	requestedCorrectCount int,
	cfg GridConfig,
	difficulty Difficulty,
	explicitCorrect []GridImageMeta,
	explicitDistractors []GridImageMeta,
) ([]GridImageMeta, map[string]struct{}, error) {
	correctCount := requestedCorrectCount
	if len(explicitCorrect) > 0 {
		if correctCount > 0 && correctCount != len(explicitCorrect) {
			return nil, nil, gridImageRequestError(
				"correct_count=%d 与 correct_image_ids 数量=%d 不一致",
				correctCount,
				len(explicitCorrect),
			)
		}
		correctCount = len(explicitCorrect)
	}
	if correctCount == 0 {
		correctCount = cfg.correctPickCount(rng)
		if correctCount > imageCount {
			correctCount = imageCount
		}
	}
	if correctCount < 1 || correctCount > imageCount {
		return nil, nil, gridImageRequestError("correct_count 必须在 1~image_count，当前=%d", correctCount)
	}

	selected := make(map[string]struct{}, imageCount)
	correct := make([]GridImageMeta, 0, correctCount)
	for _, img := range explicitCorrect {
		id := globalGridImageID(img)
		if !hasTag(img.Tags, tag) {
			return nil, nil, gridImageRequestError("正确图片 %s 不包含目标标签 %s", id, tag)
		}
		if _, exists := selected[id]; exists {
			return nil, nil, gridImageRequestError("图片重复: %s", id)
		}
		selected[id] = struct{}{}
		correct = append(correct, img)
	}

	correctCandidates := e.idx.GetGridImagesByTag(tag)
	availableCorrect := filterUnselectedGridImages(correctCandidates, selected)
	needCorrect := correctCount - len(correct)
	if len(availableCorrect) < needCorrect {
		return nil, nil, gridImageRequestError(
			"目标标签 %s 的正确图片不足：需要 %d，可用 %d",
			tag,
			correctCount,
			len(correct)+len(availableCorrect),
		)
	}
	for _, img := range pickUniqueGrid(rng, availableCorrect, needCorrect) {
		id := globalGridImageID(img)
		selected[id] = struct{}{}
		correct = append(correct, img)
	}

	distractorCount := imageCount - correctCount
	if len(explicitDistractors) > distractorCount {
		return nil, nil, gridImageRequestError(
			"distractor_image_ids 数量 %d 超过所需干扰项数量 %d",
			len(explicitDistractors),
			distractorCount,
		)
	}
	distractors := make([]GridImageMeta, 0, distractorCount)
	for _, img := range explicitDistractors {
		id := globalGridImageID(img)
		if hasTag(img.Tags, tag) {
			return nil, nil, gridImageRequestError("干扰图片 %s 包含目标标签 %s", id, tag)
		}
		if _, exists := selected[id]; exists {
			return nil, nil, gridImageRequestError("图片重复: %s", id)
		}
		selected[id] = struct{}{}
		distractors = append(distractors, img)
	}

	availableAll := filterUnselectedGridImages(e.idx.AllGridImages(), selected)
	needDistractors := distractorCount - len(distractors)
	pickedDistractors := e.pickDistractorsWithRNG(rng, tag, needDistractors, difficulty, availableAll)
	if len(pickedDistractors) < needDistractors {
		return nil, nil, gridImageRequestError(
			"Grid 干扰图片不足：需要 %d，可用 %d，tag=%s",
			distractorCount,
			len(distractors)+len(pickedDistractors),
			tag,
		)
	}
	distractors = append(distractors, pickedDistractors...)

	correctSet := make(map[string]struct{}, len(correct))
	for _, img := range correct {
		correctSet[globalGridImageID(img)] = struct{}{}
	}
	chosen := make([]GridImageMeta, 0, imageCount)
	chosen = append(chosen, correct...)
	chosen = append(chosen, distractors...)
	return chosen, correctSet, nil
}

func (e *Engine) resolveGridImageIDs(ids []string) ([]GridImageMeta, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	resolved := make([]GridImageMeta, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, gridImageRequestError("image id 不能为空")
		}
		img, err := e.resolveGridImageID(id)
		if err != nil {
			return nil, err
		}
		canonical := globalGridImageID(img)
		if _, exists := seen[canonical]; exists {
			return nil, gridImageRequestError("图片重复: %s", canonical)
		}
		seen[canonical] = struct{}{}
		resolved = append(resolved, img)
	}
	return resolved, nil
}

func (e *Engine) resolveGridImageID(id string) (GridImageMeta, error) {
	if img, ok := e.idx.GetGridImage(id); ok {
		return img, nil
	}
	if strings.Contains(id, ":") {
		return GridImageMeta{}, gridImageRequestError("Grid 图片不存在: %s", id)
	}
	matches := e.idx.GetGridImagesByBareID(id)
	if len(matches) == 0 {
		return GridImageMeta{}, gridImageRequestError("Grid 图片不存在: %s", id)
	}
	if len(matches) > 1 {
		return GridImageMeta{}, gridImageRequestError("Grid 图片 ID 不唯一，请使用 pack:image 格式: %s", id)
	}
	return matches[0], nil
}

func resolveRequestedImageCount(req GridImageGenerateRequest, configured, explicitCount int) (int, error) {
	if req.ImageCount < 0 || req.Size < 0 {
		return 0, gridImageRequestError("image_count 和 size 不能为负数")
	}
	if req.ImageCount > 0 && req.Size > 0 && req.ImageCount != req.Size {
		return 0, gridImageRequestError("image_count 与 size 不一致")
	}
	requested := req.ImageCount
	if requested == 0 {
		requested = req.Size
	}
	if explicitCount > 0 {
		if requested > 0 && requested != explicitCount {
			return 0, gridImageRequestError("image_count=%d 与 image_ids 数量=%d 不一致", requested, explicitCount)
		}
		return explicitCount, nil
	}
	if requested > 0 {
		return requested, nil
	}
	if configured > 0 {
		return configured, nil
	}
	return 9, nil
}

func validateExplicitGridImages(
	tag string,
	images []GridImageMeta,
	explicitCorrect []GridImageMeta,
	requestedCorrectCount int,
) (map[string]struct{}, error) {
	imageSet := make(map[string]GridImageMeta, len(images))
	for _, img := range images {
		imageSet[globalGridImageID(img)] = img
	}

	correctSet := make(map[string]struct{})
	if len(explicitCorrect) > 0 {
		for _, img := range explicitCorrect {
			id := globalGridImageID(img)
			if _, exists := imageSet[id]; !exists {
				return nil, gridImageRequestError("正确图片 %s 不在 image_ids 中", id)
			}
			correctSet[id] = struct{}{}
		}
	} else {
		for _, img := range images {
			if hasTag(img.Tags, tag) {
				correctSet[globalGridImageID(img)] = struct{}{}
			}
		}
	}

	if len(correctSet) == 0 {
		return nil, gridImageRequestError("image_ids 中没有包含目标标签 %s 的正确图片", tag)
	}
	if requestedCorrectCount > 0 && requestedCorrectCount != len(correctSet) {
		return nil, gridImageRequestError(
			"correct_count=%d 与实际正确图片数量=%d 不一致",
			requestedCorrectCount,
			len(correctSet),
		)
	}

	for _, img := range images {
		id := globalGridImageID(img)
		_, markedCorrect := correctSet[id]
		containsTag := hasTag(img.Tags, tag)
		if markedCorrect && !containsTag {
			return nil, gridImageRequestError("正确图片 %s 不包含目标标签 %s", id, tag)
		}
		if !markedCorrect && containsTag {
			return nil, gridImageRequestError("干扰图片 %s 也包含目标标签 %s", id, tag)
		}
	}
	return correctSet, nil
}

func imagesAtNumbers(images []GridImageMeta, numbers []int) ([]GridImageMeta, error) {
	seen := make(map[int]struct{}, len(numbers))
	out := make([]GridImageMeta, 0, len(numbers))
	for _, number := range numbers {
		if number < 1 || number > len(images) {
			return nil, gridImageRequestError("correct_numbers 中的编号 %d 超出 1~%d", number, len(images))
		}
		if _, exists := seen[number]; exists {
			return nil, gridImageRequestError("correct_numbers 包含重复编号: %d", number)
		}
		seen[number] = struct{}{}
		out = append(out, images[number-1])
	}
	return out, nil
}

func sameGridImageSet(a, b []GridImageMeta) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, img := range a {
		set[globalGridImageID(img)] = struct{}{}
	}
	for _, img := range b {
		if _, ok := set[globalGridImageID(img)]; !ok {
			return false
		}
	}
	return true
}

func commonGridTag(images []GridImageMeta) (string, error) {
	if len(images) == 0 {
		return "", gridImageRequestError("无法从空的正确图片列表推导 tag")
	}
	common := make(map[string]struct{}, len(images[0].Tags))
	for _, tag := range images[0].Tags {
		if strings.TrimSpace(tag) != "" {
			common[tag] = struct{}{}
		}
	}
	for _, img := range images[1:] {
		current := make(map[string]struct{}, len(img.Tags))
		for _, tag := range img.Tags {
			current[tag] = struct{}{}
		}
		for tag := range common {
			if _, ok := current[tag]; !ok {
				delete(common, tag)
			}
		}
	}
	if len(common) == 0 {
		return "", gridImageRequestError("correct_image_ids 没有共同标签，请显式指定 tag")
	}
	tags := make([]string, 0, len(common))
	for tag := range common {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags[0], nil
}

func filterUnselectedGridImages(images []GridImageMeta, selected map[string]struct{}) []GridImageMeta {
	out := make([]GridImageMeta, 0, len(images))
	for _, img := range images {
		if _, exists := selected[globalGridImageID(img)]; !exists {
			out = append(out, img)
		}
	}
	return out
}

func globalGridImageID(img GridImageMeta) string {
	if img.PackID == "" {
		return img.ID
	}
	return img.PackID + ":" + img.ID
}

func resolveGridLayout(imageCount, rows, columns, colsAlias int) (int, int, error) {
	if columns > 0 && colsAlias > 0 && columns != colsAlias {
		return 0, 0, gridImageRequestError("columns 与 cols 不一致")
	}
	if columns == 0 {
		columns = colsAlias
	}
	if rows < 0 || columns < 0 {
		return 0, 0, gridImageRequestError("rows 和 columns 不能为负数")
	}
	if rows > maxGeneratedImageCount || columns > maxGeneratedImageCount {
		return 0, 0, gridImageRequestError("rows/columns 不能超过 %d", maxGeneratedImageCount)
	}

	switch {
	case rows > 0 && columns > 0:
		capacity := int64(rows) * int64(columns)
		if capacity < int64(imageCount) {
			return 0, 0, gridImageRequestError("rows*columns=%d 小于 image_count=%d", capacity, imageCount)
		}
	case rows > 0:
		columns = int(math.Ceil(float64(imageCount) / float64(rows)))
	case columns > 0:
		rows = int(math.Ceil(float64(imageCount) / float64(columns)))
	default:
		rows = int(math.Ceil(math.Sqrt(float64(imageCount))))
		columns = int(math.Ceil(float64(imageCount) / float64(rows)))
	}

	if rows < 1 || columns < 1 || rows > maxGeneratedImageCount || columns > maxGeneratedImageCount {
		return 0, 0, gridImageRequestError("rows/columns 不合法: %dx%d", rows, columns)
	}
	return rows, columns, nil
}

func normalizeGridRenderOptions(
	req GridImageGenerateRequest,
	rows, columns int,
) (render.GridComposeOptions, int, bool, int, error) {
	tileWidth := req.TileWidth
	if tileWidth == 0 {
		tileWidth = defaultGeneratedTileSize
	}
	tileHeight := req.TileHeight
	if tileHeight == 0 {
		tileHeight = defaultGeneratedTileSize
	}
	if tileWidth < 16 || tileWidth > 2048 || tileHeight < 16 || tileHeight > 2048 {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("tile_width/tile_height 必须在 16~2048")
	}
	if req.Gap < 0 || req.Gap > 256 || req.Padding < 0 || req.Padding > 512 {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("gap 必须在 0~256，padding 必须在 0~512")
	}

	fit := strings.ToLower(strings.TrimSpace(req.Fit))
	if fit == "" {
		fit = render.GridFitCover
	}
	if fit != render.GridFitCover && fit != render.GridFitContain && fit != render.GridFitStretch {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("fit 必须为 cover / contain / stretch")
	}

	showLabels := true
	if req.ShowLabels != nil {
		showLabels = *req.ShowLabels
	}
	labelScale := req.LabelScale
	if labelScale == 0 {
		// 未显式指定时按 tile 尺寸自动缩放，避免大 tile 上编号过小。
		tileMin := tileWidth
		if tileHeight < tileMin {
			tileMin = tileHeight
		}
		labelScale = autoLabelScale(tileMin)
	}
	if labelScale < 1 || labelScale > 48 {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("label_scale 必须在 1~48（0=自动按 tile 尺寸）")
	}
	labelPosition := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(req.LabelPosition), "-", "_"))
	if labelPosition == "" {
		labelPosition = render.LabelTopLeft
	}
	switch labelPosition {
	case render.LabelTopLeft, render.LabelTopRight, render.LabelBottomLeft, render.LabelBottomRight, render.LabelCenter:
	default:
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError(
			"label_position 必须为 top_left / top_right / bottom_left / bottom_right / center",
		)
	}

	background, err := parseGridColor(req.Background, color.RGBA{R: 242, G: 242, B: 242, A: 255})
	if err != nil {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("background: %v", err)
	}
	labelColor, err := parseGridColor(req.LabelColor, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("label_color: %v", err)
	}
	labelBackground, err := parseGridColor(req.LabelBackground, color.RGBA{R: 0, G: 0, B: 0, A: 190})
	if err != nil {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("label_background: %v", err)
	}

	width64 := int64(req.Padding)*2 + int64(columns)*int64(tileWidth) + int64(columns-1)*int64(req.Gap)
	height64 := int64(req.Padding)*2 + int64(rows)*int64(tileHeight) + int64(rows-1)*int64(req.Gap)
	if width64 <= 0 || height64 <= 0 || width64 > maxGeneratedDimension || height64 > maxGeneratedDimension {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError(
			"生成图片尺寸超限: %dx%d，单边最大 %d",
			width64,
			height64,
			maxGeneratedDimension,
		)
	}
	if width64*height64 > maxGeneratedPixels {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError(
			"生成图片像素数超限: %d，最大 %d",
			width64*height64,
			maxGeneratedPixels,
		)
	}

	quality := req.Quality
	if quality == 0 {
		quality = 80
	}
	if quality < 1 || quality > 100 {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("quality 必须在 1~100")
	}
	applyRenderer := true
	if req.ApplyRenderer != nil {
		applyRenderer = *req.ApplyRenderer
	}
	if req.TemporaryTTLSeconds < 0 || req.TemporaryTTLSeconds > 86400 {
		return render.GridComposeOptions{}, 0, false, 0, gridImageRequestError("temporary_ttl_seconds 必须在 1~86400，或为 0 使用默认 TTL")
	}

	return render.GridComposeOptions{
		Rows:            rows,
		Columns:         columns,
		TileWidth:       tileWidth,
		TileHeight:      tileHeight,
		Gap:             req.Gap,
		Padding:         req.Padding,
		Fit:             fit,
		Background:      background,
		ShowLabels:      showLabels,
		LabelScale:      labelScale,
		LabelPosition:   labelPosition,
		LabelForeground: labelColor,
		LabelBackground: labelBackground,
	}, quality, applyRenderer, req.TemporaryTTLSeconds, nil
}

func parseGridColor(value string, fallback color.RGBA) (color.RGBA, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "black":
		return color.RGBA{A: 255}, nil
	case "white":
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}, nil
	case "transparent":
		return color.RGBA{}, nil
	}
	if !strings.HasPrefix(value, "#") {
		return color.RGBA{}, fmt.Errorf("颜色必须是 #RGB、#RGBA、#RRGGBB 或 #RRGGBBAA")
	}
	hex := strings.TrimPrefix(value, "#")
	if len(hex) == 3 || len(hex) == 4 {
		expanded := strings.Builder{}
		for _, r := range hex {
			expanded.WriteRune(r)
			expanded.WriteRune(r)
		}
		hex = expanded.String()
	}
	if len(hex) != 6 && len(hex) != 8 {
		return color.RGBA{}, fmt.Errorf("颜色长度不合法")
	}
	parsed, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("颜色值不合法")
	}
	if len(hex) == 6 {
		return color.RGBA{
			R: uint8(parsed >> 16),
			G: uint8(parsed >> 8),
			B: uint8(parsed),
			A: 255,
		}, nil
	}
	return color.RGBA{
		R: uint8(parsed >> 24),
		G: uint8(parsed >> 16),
		B: uint8(parsed >> 8),
		A: uint8(parsed),
	}, nil
}

func formatGridColor(value color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", value.R, value.G, value.B, value.A)
}

// autoLabelScale 按 tile 短边推导编号缩放系数：让数字高度约占 tile 短边的 13%
// （与 160px 默认 tile + label_scale=3 的历史观感一致），随 tile 变大自动放大。
// 数字字形为 5×7 位图，每单位 scale 令字形高度为 7*scale。
func autoLabelScale(tileMin int) int {
	if tileMin <= 0 {
		return 3
	}
	s := tileMin / 53
	if s < 3 {
		s = 3
	}
	if s > 48 {
		s = 48
	}
	return s
}

// mixSeed 在未显式指定 seed 时混入 crypto 熵，避免高并发下纳秒种子碰撞导致选图重复。
func mixSeed(seed *int64) int64 {
	if seed != nil {
		return *seed
	}
	var b [8]byte
	_, _ = crand.Read(b[:])
	s := int64(binary.BigEndian.Uint64(b[:]))
	if s == 0 {
		s = time.Now().UnixNano()
	}
	return s
}
