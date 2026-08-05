//go:build webp

package render

import (
	"bytes"
	"fmt"
	"image"

	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
)

// EncodeWebP 编码为 WebP。cgo 构建（Docker 生产，libwebp-dev）走系统 libwebp，
// 速度与质量优于纯 Go 路径；method 为 effort（0=最快，6=最慢/质量最高）。
// 必须经 NewLossyEncoderOptions 构造（先分配并初始化 C.WebPConfig），
// 再覆盖字段——直接传 struct literal 会在 GetConfig 里空指针解引用。
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

	opts, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, float32(quality))
	if err != nil {
		return nil, fmt.Errorf("webp encode 配置失败: %w", err)
	}
	opts.Method = method

	buf := &bytes.Buffer{}
	if err := webp.Encode(buf, img, opts); err != nil {
		return nil, fmt.Errorf("webp encode 失败: %w", err)
	}
	return buf.Bytes(), nil
}
