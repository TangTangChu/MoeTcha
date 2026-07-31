package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type DirectoryProvider struct {
	BaseDir      string
	MetaFileName string
	Strict       bool
}

var _ PackProvider = (*DirectoryProvider)(nil)

func (p *DirectoryProvider) LoadPacks() ([]Pack, error) {
	baseDir := strings.TrimSpace(p.BaseDir)
	if baseDir == "" {
		return nil, fmt.Errorf("BaseDir 不能为空")
	}

	metaName := strings.TrimSpace(p.MetaFileName)
	if metaName == "" {
		return nil, fmt.Errorf("MetaFileName 不能为空")
	}

	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("BaseDir 转绝对路径失败: %w", err)
	}

	entries, err := os.ReadDir(baseAbs)
	if err != nil {
		return nil, fmt.Errorf("读取 packs 根目录失败 (%s): %w", baseAbs, err)
	}

	var packs []Pack
	var firstErr error

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packID := entry.Name()
		packDir := filepath.Join(baseAbs, packID)
		metaPath := filepath.Join(packDir, metaName)

		pack, err := loadOnePack(packID, packDir, metaPath)
		if err != nil {
			if p.Strict {
				return nil, err
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		packs = append(packs, *pack)
	}

	if len(packs) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("未加载到任何有效 pack（示例错误：%w）", firstErr)
		}
		return nil, fmt.Errorf("未在目录 %s 中发现任何 pack 子目录", baseAbs)
	}

	return packs, nil
}

func loadOnePack(packID, _, metaPath string) (*Pack, error) {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("pack=%s 缺少元数据文件: %s", packID, metaPath)
		}
		return nil, fmt.Errorf("pack=%s 读取元数据失败(%s): %w", packID, metaPath, err)
	}

	var pack Pack
	if err := json.Unmarshal(b, &pack); err != nil {
		return nil, fmt.Errorf("pack=%s 解析元数据失败(%s): %w", packID, metaPath, err)
	}

	pack.ID = packID

	if strings.TrimSpace(pack.PackName) == "" {
		return nil, fmt.Errorf("pack=%s pack_name 为空", packID)
	}

	if err := validateTagDefs(packID, pack.TagDefs); err != nil {
		return nil, err
	}
	if err := validateGridConfig(packID, pack.Grid); err != nil {
		return nil, err
	}
	if err := validateClickConfig(packID, pack.Click); err != nil {
		return nil, err
	}

	if err := preparePack(packID, filepath.Dir(metaPath), &pack); err != nil {
		return nil, err
	}

	return &pack, nil
}

// --- tag_defs 校验 ---

func validateTagDefs(packID string, defs map[string]TagDef) error {
	for tag, def := range defs {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return fmt.Errorf("pack=%s tag_defs 中存在空 key", packID)
		}
		if strings.TrimSpace(def.Name) == "" {
			return fmt.Errorf("pack=%s tag_defs[%s].name 为空", packID, tag)
		}
		for _, s := range def.Similar {
			if s == tag {
				return fmt.Errorf("pack=%s tag_defs[%s].similar 包含自身", packID, tag)
			}
		}
	}
	return nil
}

// --- grid / click 配置校验 ---

func validateGridConfig(packID string, g *GridConfig) error {
	if g == nil {
		return nil
	}
	if g.Size != 0 && g.Size < 4 {
		return fmt.Errorf("pack=%s grid.size 至少为 4, 当前=%d", packID, g.Size)
	}
	if g.CorrectMin != 0 && g.CorrectMin < 1 {
		return fmt.Errorf("pack=%s grid.correct_min 至少为 1, 当前=%d", packID, g.CorrectMin)
	}
	if g.CorrectMax != 0 && g.CorrectMin != 0 && g.CorrectMax < g.CorrectMin {
		return fmt.Errorf("pack=%s grid.correct_max(%d) 不能小于 correct_min(%d)", packID, g.CorrectMax, g.CorrectMin)
	}
	if g.Size != 0 && g.CorrectMax != 0 && g.CorrectMax >= g.Size {
		return fmt.Errorf("pack=%s grid.correct_max(%d) 必须小于 grid.size(%d)", packID, g.CorrectMax, g.Size)
	}
	return nil
}

func validateClickConfig(packID string, c *ClickConfig) error {
	if c == nil {
		return nil
	}
	if c.Count < 0 {
		return fmt.Errorf("pack=%s click.count 不能为负数", packID)
	}
	return nil
}

// --- 素材准备 ---

func preparePack(packID, packDir string, pack *Pack) error {
	seen := make(map[string]struct{})

	for i := range pack.GridImages {
		g := &pack.GridImages[i]

		file := strings.TrimSpace(g.File)
		if file == "" {
			return fmt.Errorf("pack=%s grid_images[%d].file 为空", packID, i)
		}

		id := fileStem(file)
		if id == "" {
			return fmt.Errorf("pack=%s grid_images[%d].file 非法: %q", packID, i, file)
		}

		if _, ok := seen[id]; ok {
			return fmt.Errorf("pack=%s 图片ID冲突（由文件名推导）: %s", packID, id)
		}
		seen[id] = struct{}{}

		if len(g.Tags) == 0 {
			return fmt.Errorf("pack=%s grid_images[%d] tags 为空, file=%q", packID, i, file)
		}

		path := filepath.Join(packDir, filepath.Base(file))
		if err := mustExistFile(path); err != nil {
			return fmt.Errorf("pack=%s grid_images[%d] 图片文件不存在: %s: %w", packID, i, path, err)
		}

		g.File = filepath.Base(file)
		g.ID = id
		g.PackID = packID
		g.Path = path
	}

	for i := range pack.ClickImages {
		c := &pack.ClickImages[i]

		file := strings.TrimSpace(c.File)
		if file == "" {
			return fmt.Errorf("pack=%s click_images[%d].file 为空", packID, i)
		}

		id := fileStem(file)
		if id == "" {
			return fmt.Errorf("pack=%s click_images[%d].file 非法: %q", packID, i, file)
		}

		if _, ok := seen[id]; ok {
			return fmt.Errorf("pack=%s 图片ID冲突（由文件名推导）: %s", packID, id)
		}
		seen[id] = struct{}{}

		if len(c.Regions) == 0 {
			return fmt.Errorf("pack=%s click_images[%d] regions 为空, file=%q", packID, i, file)
		}

		for ri, r := range c.Regions {
			if strings.TrimSpace(r.Tag) == "" {
				return fmt.Errorf("pack=%s click_images[%d] regions[%d].tag 为空", packID, i, ri)
			}
			if r.X < 0 || r.Y < 0 || r.Width <= 0 || r.Height <= 0 {
				return fmt.Errorf(
					"pack=%s click_images[%d] regions[%d] 矩形非法 x=%d y=%d w=%d h=%d",
					packID, i, ri, r.X, r.Y, r.Width, r.Height,
				)
			}
		}

		path := filepath.Join(packDir, filepath.Base(file))
		if err := mustExistFile(path); err != nil {
			return fmt.Errorf("pack=%s click_images[%d] 图片文件不存在: %s: %w", packID, i, path, err)
		}

		c.File = filepath.Base(file)
		c.ID = id
		c.PackID = packID
		c.Path = path
	}

	return nil
}

func fileStem(file string) string {
	base := filepath.Base(file)
	ext := filepath.Ext(base)
	stem := strings.TrimSpace(strings.TrimSuffix(base, ext))
	if stem == "" || stem == "." || stem == ".." {
		return ""
	}
	return stem
}

func mustExistFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("路径是目录而不是文件: %s", path)
	}
	return nil
}
