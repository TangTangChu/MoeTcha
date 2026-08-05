//go:build !webp

package render

import (
	"bytes"
	"fmt"
	"image"

	genwebp "github.com/gen2brain/webp"
)

// EncodeWebP 编码为 WebP。纯 Go 构建（无 cgo/系统 libwebp，如 Windows 开发机）
// 走 gen2brain 编码器（libwebp 的纯 Go/WASM 移植），产物与 cgo 版一致。
// method 为 effort（0=最快，6=最慢/质量最高）；越界回退 4（libwebp 默认）。
func EncodeWebP(img image.Image, quality int, method int) ([]byte, error) {
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
