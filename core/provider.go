package core

// TagDef 标签定义：控制标签的显示名与相似标签（用于难度）。
type TagDef struct {
	Name    string   `json:"name"`              // 显示名，如 "白毛"
	Similar []string `json:"similar,omitempty"` // 相似标签列表，用于难度控制
}

// GridConfig 网格挑战的 pack 级配置。
type GridConfig struct {
	Size       int    `json:"size"`               // 网格图片总数，默认 9
	CorrectMin int    `json:"correct_min"`        // 最少正确数，默认 2
	CorrectMax int    `json:"correct_max"`        // 最多正确数，默认 4
	Question   string `json:"question,omitempty"` // 问题模板，{tag} → 标签显示名
}

// ClickConfig 点击挑战的 pack 级配置。
type ClickConfig struct {
	Question string `json:"question,omitempty"` // 问题模板，{tag} → 标签显示名
}

type GridImageMeta struct {
	File string   `json:"file"`
	Tags []string `json:"tags"`

	ID     string `json:"-"`
	PackID string `json:"-"`
	Path   string `json:"-"`
}

// Region 描述图片中的一个可点击区域。
type Region struct {
	Tag    string `json:"tag"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// ClickImageMeta 描述一张用于区域点选的图片。
type ClickImageMeta struct {
	File    string   `json:"file"`
	Regions []Region `json:"regions"`

	ID     string `json:"-"`
	PackID string `json:"-"`
	Path   string `json:"-"`
}

type Pack struct {
	ID          string            `json:"-"`
	PackName    string            `json:"pack_name"`
	Author      string            `json:"author,omitempty"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	TagDefs     map[string]TagDef `json:"tag_defs,omitempty"` // 标签定义表
	Grid        *GridConfig       `json:"grid,omitempty"`     // 网格挑战配置
	Click       *ClickConfig      `json:"click,omitempty"`    // 点击挑战配置
	GridImages  []GridImageMeta   `json:"grid_images"`
	ClickImages []ClickImageMeta  `json:"click_images"`
	Extra       map[string]any    `json:"extra,omitempty"`
}

type PackProvider interface {
	LoadPacks() ([]Pack, error)
}

// Difficulty 控制干扰项选择策略。
type Difficulty string

const (
	DiffEasy   Difficulty = "easy"
	DiffMedium Difficulty = "medium"
	DiffHard   Difficulty = "hard"
)

// defaultGridConfig 返回网格挑战默认配置。
func defaultGridConfig() GridConfig {
	return GridConfig{
		Size:       9,
		CorrectMin: 2,
		CorrectMax: 4,
		Question:   "请选出所有「{tag}」",
	}
}

// defaultClickConfig 返回点击挑战默认配置。
func defaultClickConfig() ClickConfig {
	return ClickConfig{
		Question: "请点击图中所有「{tag}」",
	}
}

// ResolvedGridConfig 用 pack 级设置覆盖默认值后返回。
func (p Pack) ResolvedGridConfig() GridConfig {
	cfg := defaultGridConfig()
	if p.Grid != nil {
		if p.Grid.Size > 0 {
			cfg.Size = p.Grid.Size
		}
		if p.Grid.CorrectMin > 0 {
			cfg.CorrectMin = p.Grid.CorrectMin
		}
		if p.Grid.CorrectMax > 0 {
			cfg.CorrectMax = p.Grid.CorrectMax
		}
		if p.Grid.Question != "" {
			cfg.Question = p.Grid.Question
		}
	}
	return cfg
}

// ResolvedClickConfig 用 pack 级设置覆盖默认值后返回。
func (p Pack) ResolvedClickConfig() ClickConfig {
	cfg := defaultClickConfig()
	if p.Click != nil {
		if p.Click.Question != "" {
			cfg.Question = p.Click.Question
		}
	}
	return cfg
}
