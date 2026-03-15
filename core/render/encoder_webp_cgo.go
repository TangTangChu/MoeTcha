//go:build webp

package render

import (
	"bytes"
	"fmt"
	"image"

	"github.com/kolesa-team/go-webp/webp"
)

func EncodeWebP(img image.Image, quality float32) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("image 为空")
	}
	if quality <= 0 || quality > 100 {
		quality = 80
	}
	buf := &bytes.Buffer{}
	if err := webp.Encode(buf, img, &webp.Options{Lossless: false, Quality: quality}); err != nil {
		return nil, fmt.Errorf("webp encode 失败: %w", err)
	}
	return buf.Bytes(), nil
}

func ContentType() string {
	return "image/webp"
}
