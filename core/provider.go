package core

type GridImageMeta struct {
	File string   `json:"file"`
	Tags []string `json:"tags"`

	ID     string `json:"-"`
	PackID string `json:"-"`
	Path   string `json:"-"`
}

// Region 描述图片中的一个可点击区域
type Region struct {
	Tag    string `json:"tag"` // 该区域的标签
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`  // 区域宽度
	Height int    `json:"height"` // 区域高度
}

// ClickImageMeta 描述一张用于区域点选的图片
type ClickImageMeta struct {
	File    string   `json:"file"`
	Regions []Region `json:"regions"`

	ID     string `json:"-"`
	PackID string `json:"-"`
	Path   string `json:"-"`
}

type Pack struct {
	ID          string           `json:"-"`
	PackName    string           `json:"pack_name"`
	Author      string           `json:"author,omitempty"`
	Version     string           `json:"version,omitempty"`
	Description string           `json:"description,omitempty"`
	GridImages  []GridImageMeta  `json:"grid_images"`
	ClickImages []ClickImageMeta `json:"click_images"`
	Extra       map[string]any   `json:"extra,omitempty"`
}

type PackProvider interface {
	LoadPacks() ([]Pack, error)
}
