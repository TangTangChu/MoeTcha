package render

import (
	"image"
	"image/color"
	"image/draw"
	"math/rand"
	"time"
)

type NoiseObfuscator struct {
	Density float64
	Seed    int64
}

func (n NoiseObfuscator) Apply(img image.Image) (image.Image, error) {
	if img == nil {
		return nil, nil
	}
	// Density<=0 表示关闭噪声：直接返回原图，不做任何改动。
	// 这让 RENDER_NOISE_ENABLED=false（或 density=0）能真正产出干净的高质量图。
	if n.Density <= 0 {
		return img, nil
	}

	seed := n.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)

	// 上限 0.2 防止密度过大导致图片不可辨认；下限已在上面以 no-op 处理。
	density := n.Density
	if density > 0.2 {
		density = 0.2
	}

	total := b.Dx() * b.Dy()
	points := int(float64(total) * density)
	for i := 0; i < points; i++ {
		x := b.Min.X + rng.Intn(b.Dx())
		y := b.Min.Y + rng.Intn(b.Dy())
		c := color.RGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 255}
		out.Set(x, y, c)
	}

	return out, nil
}
