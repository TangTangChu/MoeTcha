package render

import "net/http"

// ContentTypeForBytes 根据实际内容判断图片格式，避免 build tag 与存储内容不一致。
func ContentTypeForBytes(data []byte) string {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	return http.DetectContentType(data)
}
