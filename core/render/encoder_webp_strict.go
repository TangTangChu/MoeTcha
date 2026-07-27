package render

import (
	"bytes"
	"fmt"
	"image"

	genwebp "github.com/gen2brain/webp"
)

// EncodeWebPStrict 始终编码为 WebP，不受 legacy webp build tag 影响。
func EncodeWebPStrict(img image.Image, quality int) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("image 为空")
	}
	if quality <= 0 || quality > 100 {
		quality = 80
	}

	buf := &bytes.Buffer{}
	if err := genwebp.Encode(buf, img, genwebp.Options{Quality: quality, Method: 4}); err != nil {
		return nil, fmt.Errorf("webp encode 失败: %w", err)
	}
	return buf.Bytes(), nil
}
