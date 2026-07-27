package render

import (
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

const (
	GridFitCover   = "cover"
	GridFitContain = "contain"
	GridFitStretch = "stretch"

	LabelTopLeft     = "top_left"
	LabelTopRight    = "top_right"
	LabelBottomLeft  = "bottom_left"
	LabelBottomRight = "bottom_right"
	LabelCenter      = "center"
)

type GridComposeOptions struct {
	Rows            int
	Columns         int
	TileWidth       int
	TileHeight      int
	Gap             int
	Padding         int
	Fit             string
	Background      color.RGBA
	ShowLabels      bool
	LabelScale      int
	LabelPosition   string
	LabelForeground color.RGBA
	LabelBackground color.RGBA
}

type GridPlacement struct {
	Number int
	X      int
	Y      int
	Width  int
	Height int
}

func ComposeGrid(images []image.Image, opts GridComposeOptions) (*image.RGBA, []GridPlacement, error) {
	if opts.Rows <= 0 || opts.Columns <= 0 {
		return nil, nil, fmt.Errorf("rows 和 columns 必须大于 0")
	}
	if opts.TileWidth <= 0 || opts.TileHeight <= 0 {
		return nil, nil, fmt.Errorf("tile width 和 height 必须大于 0")
	}
	if opts.Gap < 0 || opts.Padding < 0 {
		return nil, nil, fmt.Errorf("gap 和 padding 不能为负数")
	}
	if len(images) > opts.Rows*opts.Columns {
		return nil, nil, fmt.Errorf("图片数量 %d 超过布局容量 %d", len(images), opts.Rows*opts.Columns)
	}

	if opts.Fit == "" {
		opts.Fit = GridFitCover
	}
	switch opts.Fit {
	case GridFitCover, GridFitContain, GridFitStretch:
	default:
		return nil, nil, fmt.Errorf("未知 fit 模式: %s", opts.Fit)
	}
	if opts.LabelScale <= 0 {
		opts.LabelScale = 3
	}
	if opts.LabelPosition == "" {
		opts.LabelPosition = LabelTopLeft
	}
	switch opts.LabelPosition {
	case LabelTopLeft, LabelTopRight, LabelBottomLeft, LabelBottomRight, LabelCenter:
	default:
		return nil, nil, fmt.Errorf("未知 label_position: %s", opts.LabelPosition)
	}

	width := opts.Padding*2 + opts.Columns*opts.TileWidth + (opts.Columns-1)*opts.Gap
	height := opts.Padding*2 + opts.Rows*opts.TileHeight + (opts.Rows-1)*opts.Gap
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(canvas, canvas.Bounds(), image.NewUniform(opts.Background), image.Point{}, stddraw.Src)

	placements := make([]GridPlacement, 0, len(images))
	for i, src := range images {
		if src == nil {
			return nil, nil, fmt.Errorf("第 %d 张图片为空", i+1)
		}
		row := i / opts.Columns
		col := i % opts.Columns
		x := opts.Padding + col*(opts.TileWidth+opts.Gap)
		y := opts.Padding + row*(opts.TileHeight+opts.Gap)
		tileRect := image.Rect(x, y, x+opts.TileWidth, y+opts.TileHeight)

		drawFitted(canvas, tileRect, src, opts.Fit, opts.Background)
		if opts.ShowLabels {
			drawNumberLabel(canvas, tileRect, i+1, opts)
		}
		placements = append(placements, GridPlacement{
			Number: i + 1,
			X:      x,
			Y:      y,
			Width:  opts.TileWidth,
			Height: opts.TileHeight,
		})
	}

	return canvas, placements, nil
}

func drawFitted(dst *image.RGBA, dstRect image.Rectangle, src image.Image, fit string, background color.RGBA) {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := dstRect.Dx(), dstRect.Dy()
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return
	}

	if fit == GridFitStretch {
		xdraw.CatmullRom.Scale(dst, dstRect, src, sb, stddraw.Over, nil)
		return
	}

	sx := float64(dw) / float64(sw)
	sy := float64(dh) / float64(sh)
	scale := sx
	if fit == GridFitCover {
		if sy > scale {
			scale = sy
		}
	} else if sy < scale {
		scale = sy
	}

	resizeW := maxInt(1, int(float64(sw)*scale+0.5))
	resizeH := maxInt(1, int(float64(sh)*scale+0.5))
	resized := image.NewRGBA(image.Rect(0, 0, resizeW, resizeH))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), src, sb, stddraw.Src, nil)

	if fit == GridFitContain {
		stddraw.Draw(dst, dstRect, image.NewUniform(background), image.Point{}, stddraw.Src)
		x := dstRect.Min.X + (dw-resizeW)/2
		y := dstRect.Min.Y + (dh-resizeH)/2
		stddraw.Draw(dst, image.Rect(x, y, x+resizeW, y+resizeH), resized, image.Point{}, stddraw.Over)
		return
	}

	sourceX := maxInt(0, (resizeW-dw)/2)
	sourceY := maxInt(0, (resizeH-dh)/2)
	stddraw.Draw(dst, dstRect, resized, image.Pt(sourceX, sourceY), stddraw.Src)
}

var digitGlyphs = map[rune][7]string{
	'0': {"11111", "10001", "10011", "10101", "11001", "10001", "11111"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"11111", "00001", "00001", "11111", "10000", "10000", "11111"},
	'3': {"11111", "00001", "00001", "01111", "00001", "00001", "11111"},
	'4': {"10001", "10001", "10001", "11111", "00001", "00001", "00001"},
	'5': {"11111", "10000", "10000", "11111", "00001", "00001", "11111"},
	'6': {"11111", "10000", "10000", "11111", "10001", "10001", "11111"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"11111", "10001", "10001", "11111", "10001", "10001", "11111"},
	'9': {"11111", "10001", "10001", "11111", "00001", "00001", "11111"},
}

func drawNumberLabel(dst *image.RGBA, tile image.Rectangle, number int, opts GridComposeOptions) {
	text := strconv.Itoa(number)
	scale := opts.LabelScale
	textWidth := len(text)*5*scale + (len(text)-1)*scale
	textHeight := 7 * scale
	boxPadding := maxInt(2, scale)
	boxWidth := textWidth + boxPadding*2
	boxHeight := textHeight + boxPadding*2
	margin := maxInt(2, scale)

	x, y := tile.Min.X+margin, tile.Min.Y+margin
	switch opts.LabelPosition {
	case LabelTopRight:
		x = tile.Max.X - margin - boxWidth
	case LabelBottomLeft:
		y = tile.Max.Y - margin - boxHeight
	case LabelBottomRight:
		x = tile.Max.X - margin - boxWidth
		y = tile.Max.Y - margin - boxHeight
	case LabelCenter:
		x = tile.Min.X + (tile.Dx()-boxWidth)/2
		y = tile.Min.Y + (tile.Dy()-boxHeight)/2
	}

	if x < tile.Min.X {
		x = tile.Min.X
	}
	if y < tile.Min.Y {
		y = tile.Min.Y
	}
	if x+boxWidth > tile.Max.X {
		boxWidth = tile.Max.X - x
	}
	if y+boxHeight > tile.Max.Y {
		boxHeight = tile.Max.Y - y
	}
	if boxWidth <= 0 || boxHeight <= 0 {
		return
	}

	box := image.Rect(x, y, x+boxWidth, y+boxHeight)
	stddraw.Draw(dst, box, image.NewUniform(opts.LabelBackground), image.Point{}, stddraw.Over)
	drawDigits(dst, x+boxPadding, y+boxPadding, text, scale, opts.LabelForeground)
}

func drawDigits(dst *image.RGBA, x, y int, text string, scale int, foreground color.RGBA) {
	cursor := x
	for _, r := range text {
		glyph, ok := digitGlyphs[r]
		if !ok {
			continue
		}
		for gy, line := range glyph {
			for gx, bit := range line {
				if bit != '1' {
					continue
				}
				cell := image.Rect(
					cursor+gx*scale,
					y+gy*scale,
					cursor+(gx+1)*scale,
					y+(gy+1)*scale,
				)
				stddraw.Draw(dst, cell, image.NewUniform(foreground), image.Point{}, stddraw.Over)
			}
		}
		cursor += 6 * scale
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
