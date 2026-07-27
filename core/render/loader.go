package render

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	genwebp "github.com/gen2brain/webp"
)

func LoadImage(path string) (image.Image, error) {
	if path == "" {
		return nil, fmt.Errorf("image path 为空")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()

	var img image.Image
	if strings.EqualFold(filepath.Ext(path), ".webp") {
		img, err = genwebp.Decode(f)
	} else {
		img, _, err = image.Decode(f)
	}
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %w", err)
	}
	return img, nil
}
