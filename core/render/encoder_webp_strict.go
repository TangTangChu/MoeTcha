package render

import (
	"bytes"
	"fmt"
	"image"

	genwebp "github.com/gen2brain/webp"
)

// EncodeWebPStrict 始终编码为 WebP，不受 legacy webp build tag 影响。
// method 为 libwebp effort（0=最快，6=最慢/质量最高）；超出 0~6 时回退 4（libwebp 默认）。
func EncodeWebPStrict(img image.Image, quality int, method int) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("image 为空")
	}
	if quality <= 0 || quality > 100 {
		quality = 80
	}
	if method < 0 || method > 6 {
		method = 4
	}

	buf := &bytes.Buffer{}
	if err := genwebp.Encode(buf, img, genwebp.Options{Quality: quality, Method: method}); err != nil {
		return nil, fmt.Errorf("webp encode 失败: %w", err)
	}
	return buf.Bytes(), nil
}
