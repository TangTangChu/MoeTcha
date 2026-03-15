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

	seed := n.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)

	density := n.Density
	if density <= 0 {
		density = 0.02
	}
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
