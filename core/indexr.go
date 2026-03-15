package core

import (
	"fmt"
)

type Indexer struct {
	gridImages   map[string]GridImageMeta
	gridTagIndex map[string][]string

	clickImages   map[string]ClickImageMeta
	clickTagIndex map[string][]string
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

func globalImageID(packID, imageID string) string {
	return packID + ":" + imageID
}

func (idx *Indexer) GetAllGridTags() []string {
	tags := make([]string, 0, len(idx.gridTagIndex))
	for tag := range idx.gridTagIndex {
		tags = append(tags, tag)
	}
	return tags
}

func (idx *Indexer) GetAllClickTags() []string {
	tags := make([]string, 0, len(idx.clickTagIndex))
	for tag := range idx.clickTagIndex {
		tags = append(tags, tag)
	}
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
	return out
}

func (idx *Indexer) AllClickImages() []ClickImageMeta {
	out := make([]ClickImageMeta, 0, len(idx.clickImages))
	for _, img := range idx.clickImages {
		out = append(out, img)
	}
	return out
}
