package render

import (
	"image"
	"image/color"
	"testing"
)

// TestComposeGridDecimatesLargeSource ensures the two-pass (halve + CatmullRom)
// downscale path still composes a correctly-sized canvas and fills tiles with the
// source color (no empty/black tiles), exercising the big-downscale branch.
func TestComposeGridDecimatesLargeSource(t *testing.T) {
	// 1800x1200 source (well above the 2x decimation threshold for 200x200 tiles).
	src := image.NewRGBA(image.Rect(0, 0, 1800, 1200))
	fill := color.RGBA{R: 200, G: 30, B: 90, A: 255}
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetRGBA(x, y, fill)
		}
	}
	images := make([]image.Image, 4)
	for i := range images {
		images[i] = src
	}
	opts := GridComposeOptions{
		Rows: 2, Columns: 2, TileWidth: 200, TileHeight: 200, Gap: 4, Padding: 4,
		Fit: GridFitCover, Background: color.RGBA{0, 0, 0, 255},
		ShowLabels: true, LabelScale: 3,
	}
	canvas, placements, err := ComposeGrid(images, opts)
	if err != nil {
		t.Fatalf("ComposeGrid: %v", err)
	}
	// 2 tiles wide: padding*2 + 2*200 + 1*gap = 8+400+4 = 412
	if want := 412; canvas.Bounds().Dx() != want || canvas.Bounds().Dy() != want {
		t.Fatalf("canvas=%dx%d, want %dx%d", canvas.Bounds().Dx(), canvas.Bounds().Dy(), want, want)
	}
	if len(placements) != 4 {
		t.Fatalf("placements=%d, want 4", len(placements))
	}
	// Tile center should carry the source color (cover fills the tile), not the black background.
	c := canvas.At(100, 100)
	if r, g, b, _ := c.RGBA(); r>>8 < 150 || g>>8 > 80 || b>>8 > 130 {
		t.Fatalf("tile center pixel %v not source color-ish (cover fill failed)", c)
	}
}

// TestComposeGridNoDecimateForSmallSource ensures the small-downscale / upscale
// path (no decimation) still works and matches the historic direct CatmullRom flow.
func TestComposeGridNoDecimateForSmallSource(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 10, G: 200, B: 10, A: 255})
		}
	}
	opts := GridComposeOptions{
		Rows: 1, Columns: 1, TileWidth: 160, TileHeight: 160,
		Fit: GridFitStretch, Background: color.RGBA{0, 0, 0, 255},
		ShowLabels: false,
	}
	canvas, _, err := ComposeGrid([]image.Image{src}, opts)
	if err != nil {
		t.Fatalf("ComposeGrid: %v", err)
	}
	if canvas.Bounds().Dx() != 160 || canvas.Bounds().Dy() != 160 {
		t.Fatalf("canvas=%dx%d, want 160x160", canvas.Bounds().Dx(), canvas.Bounds().Dy())
	}
}

// TestComposeGridLabelBoxScalesWithTile verifies that the number label box grows
// proportionally with tile size (the fix for "numbers too small on large tiles").
func TestComposeGridLabelBoxScalesWithTile(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 30, G: 30, B: 30, A: 255})
		}
	}
	labelFG := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	labelBG := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	countLabelFG := func(tile int, scale int) int {
		opts := GridComposeOptions{
			Rows: 1, Columns: 1, TileWidth: tile, TileHeight: tile,
			Fit: GridFitCover, Background: color.RGBA{0, 0, 0, 255},
			ShowLabels: true, LabelScale: scale,
			LabelPosition: LabelTopLeft, LabelForeground: labelFG, LabelBackground: labelBG,
		}
		canvas, _, err := ComposeGrid([]image.Image{src}, opts)
		if err != nil {
			t.Fatalf("ComposeGrid: %v", err)
		}
		// count foreground (white) pixels in the tile = label glyph pixels.
		n := 0
		for y := 0; y < tile; y++ {
			for x := 0; x < tile; x++ {
				r, g, b, _ := canvas.At(x, y).RGBA()
				if r>>8 == 255 && g>>8 == 255 && b>>8 == 255 {
					n++
				}
			}
		}
		return n
	}
	// default scale 3 on 160px tile (historic), auto scale 11 on 600px tile (new).
	small := countLabelFG(160, 3)
	large := countLabelFG(600, 11)
	if large <= small*2 {
		t.Fatalf("label on 600px (auto scale 11) = %d fg px, expected ~2x+ the 160px baseline %d", large, small)
	}
}
