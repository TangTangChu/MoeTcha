package core

import (
	"fmt"
	"sort"
	"strings"
)

// packConfig 存储单个 pack 的挑战配置。
type packConfig struct {
	Grid  GridConfig
	Click ClickConfig
}

type Indexer struct {
	gridImages   map[string]GridImageMeta
	gridTagIndex map[string][]string

	clickImages   map[string]ClickImageMeta
	clickTagIndex map[string][]string

	tagDefs     map[string]TagDef // tag → 定义（跨 pack 合并，冲突时报错）
	packConfigs map[string]packConfig
}

func NewIndexer(provider PackProvider) (*Indexer, error) {
	packs, err := provider.LoadPacks()
	if err != nil {
		return nil, err
	}

	idx := &Indexer{
		gridImages:    make(map[string]GridImageMeta),
		gridTagIndex:  make(map[string][]string),
		clickImages:   make(map[string]ClickImageMeta),
		clickTagIndex: make(map[string][]string),
		tagDefs:       make(map[string]TagDef),
		packConfigs:   make(map[string]packConfig),
	}

	if err := idx.BuildFromPacks(packs); err != nil {
		return nil, err
	}

	return idx, nil
}

func (idx *Indexer) BuildFromPacks(packs []Pack) error {
	for _, pack := range packs {
		packID := pack.ID
		if packID == "" {
			return fmt.Errorf("pack 缺少 ID")
		}

		// 存储 pack 级配置
		idx.packConfigs[packID] = packConfig{
			Grid:  pack.ResolvedGridConfig(),
			Click: pack.ResolvedClickConfig(),
		}

		// 合并 tag 定义
		for tag, def := range pack.TagDefs {
			if existing, ok := idx.tagDefs[tag]; ok {
				if existing.Name != def.Name {
					return fmt.Errorf("tag=%s 在不同 pack 中 display_name 冲突: %q vs %q", tag, existing.Name, def.Name)
				}
				// similar 合并（取并集）
				merged := mergeSimilar(existing.Similar, def.Similar)
				idx.tagDefs[tag] = TagDef{Name: existing.Name, Similar: merged}
				continue
			}
			idx.tagDefs[tag] = def
		}

		for _, img := range pack.GridImages {
			gid := globalImageID(packID, img.ID)
			if _, exists := idx.gridImages[gid]; exists {
				return fmt.Errorf("Grid 图片全局ID冲突: %s", gid)
			}

			idx.gridImages[gid] = img
			for _, tag := range img.Tags {
				idx.gridTagIndex[tag] = append(idx.gridTagIndex[tag], gid)
			}
		}

		for _, img := range pack.ClickImages {
			gid := globalImageID(packID, img.ID)
			if _, exists := idx.clickImages[gid]; exists {
				return fmt.Errorf("Click 图片全局ID冲突: %s", gid)
			}

			idx.clickImages[gid] = img
			for _, region := range img.Regions {
				idx.clickTagIndex[region.Tag] = append(idx.clickTagIndex[region.Tag], gid)
			}
		}
	}

	return nil
}

func mergeSimilar(a, b []string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	return out
}

func globalImageID(packID, imageID string) string {
	return packID + ":" + imageID
}

// --- tag 查询 ---

func (idx *Indexer) TagDisplay(tag string) string {
	if def, ok := idx.tagDefs[tag]; ok {
		return def.Name
	}
	return tag
}

func (idx *Indexer) SimilarTags(tag string) []string {
	if def, ok := idx.tagDefs[tag]; ok {
		return def.Similar
	}
	return nil
}

// --- pack 配置查询：按 tag 所在 pack 中贡献图片最多的 pack 的配置 ---

func (idx *Indexer) GridConfigForTag(tag string) GridConfig {
	packID := idx.bestPackForTag(tag, idx.gridTagIndex)
	if cfg, ok := idx.packConfigs[packID]; ok {
		return cfg.Grid
	}
	return defaultGridConfig()
}

func (idx *Indexer) ClickConfigForTag(tag string) ClickConfig {
	packID := idx.bestPackForTag(tag, idx.clickTagIndex)
	if cfg, ok := idx.packConfigs[packID]; ok {
		return cfg.Click
	}
	return defaultClickConfig()
}

func (idx *Indexer) bestPackForTag(tag string, tagIndex map[string][]string) string {
	gids := tagIndex[tag]
	counts := make(map[string]int, len(gids))
	for _, gid := range gids {
		packID := extractPackID(gid)
		counts[packID]++
	}
	best, bestN := "", 0
	for pid, n := range counts {
		if n > bestN {
			best, bestN = pid, n
		}
	}
	return best
}

func extractPackID(globalID string) string {
	if i := strings.LastIndex(globalID, ":"); i >= 0 {
		return globalID[:i]
	}
	return globalID
}

// --- 原有的图像查询 ---

func (idx *Indexer) GetAllGridTags() []string {
	tags := make([]string, 0, len(idx.gridTagIndex))
	for tag := range idx.gridTagIndex {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func (idx *Indexer) GetAllClickTags() []string {
	tags := make([]string, 0, len(idx.clickTagIndex))
	for tag := range idx.clickTagIndex {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func (idx *Indexer) GetGridImagesByTag(tag string) []GridImageMeta {
	ids := idx.gridTagIndex[tag]
	imgs := make([]GridImageMeta, 0, len(ids))
	for _, id := range ids {
		img, ok := idx.gridImages[id]
		if ok {
			imgs = append(imgs, img)
		}
	}
	return imgs
}

func (idx *Indexer) GetClickImagesByTag(tag string) []ClickImageMeta {
	ids := idx.clickTagIndex[tag]
	imgs := make([]ClickImageMeta, 0, len(ids))
	for _, id := range ids {
		img, ok := idx.clickImages[id]
		if ok {
			imgs = append(imgs, img)
		}
	}
	return imgs
}

func (idx *Indexer) GetGridImage(globalID string) (GridImageMeta, bool) {
	img, ok := idx.gridImages[globalID]
	return img, ok
}

func (idx *Indexer) GetClickImage(globalID string) (ClickImageMeta, bool) {
	img, ok := idx.clickImages[globalID]
	return img, ok
}

func (idx *Indexer) AllGridImages() []GridImageMeta {
	out := make([]GridImageMeta, 0, len(idx.gridImages))
	for _, img := range idx.gridImages {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool {
		return globalImageID(out[i].PackID, out[i].ID) < globalImageID(out[j].PackID, out[j].ID)
	})
	return out
}

func (idx *Indexer) AllClickImages() []ClickImageMeta {
	out := make([]ClickImageMeta, 0, len(idx.clickImages))
	for _, img := range idx.clickImages {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool {
		return globalImageID(out[i].PackID, out[i].ID) < globalImageID(out[j].PackID, out[j].ID)
	})
	return out
}
