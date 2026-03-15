//go:build !webp

package render

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

func EncodeWebP(img image.Image, _ float32) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("image 为空")
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return nil, fmt.Errorf("png encode 失败: %w", err)
	}
	return buf.Bytes(), nil
}

func ContentType() string {
	return "image/png"
}
