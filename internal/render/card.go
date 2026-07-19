// Package render draws a bingo card to a PNG for display in Discord (Discord
// cannot render our interactive web card, so the bot attaches an image of the
// current state alongside the toggle buttons). It uses the BSD-licensed Go font
// bundled in golang.org/x/image, so no font file needs to be shipped.
package render

import (
	"bytes"
	"fmt"
	"image/png"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// Cell is one square to draw.
type Cell struct {
	Text   string
	Marked bool
	Free   bool
}

// Options controls a card render.
type Options struct {
	Title    string
	Subtitle string
	Cells    []Cell // exactly bingo.CellCount, ordered by index
}

// Layout constants (pixels).
const (
	cellSize    = 150
	gridPad     = 12
	headerH     = 44
	titleH      = 64
	cols        = bingo.Size
	fontRegular = 17.0
	fontHeader  = 24.0
	fontTitle   = 26.0
)

// colours
var (
	bgColor      = [3]float64{0.12, 0.13, 0.16}
	cellColor    = [3]float64{0.20, 0.22, 0.27}
	markedColor  = [3]float64{0.16, 0.55, 0.32}
	freeColor    = [3]float64{0.55, 0.44, 0.15}
	textColor    = [3]float64{0.93, 0.94, 0.96}
	subtleColor  = [3]float64{0.66, 0.70, 0.78}
	gridLine     = [3]float64{0.30, 0.33, 0.40}
	headerLetter = "BINGO"
)

// RenderCard draws the card and returns PNG bytes.
func RenderCard(opts Options) ([]byte, error) {
	if len(opts.Cells) != bingo.CellCount {
		return nil, fmt.Errorf("render: got %d cells, want %d", len(opts.Cells), bingo.CellCount)
	}
	font, err := truetype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("render: parse font: %w", err)
	}

	gridW := cols*cellSize + 2*gridPad
	width := gridW
	height := titleH + headerH + cols*cellSize + 2*gridPad

	dc := gg.NewContext(width, height)
	setColor(dc, bgColor)
	dc.Clear()

	// Title / subtitle.
	dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: fontTitle}))
	setColor(dc, textColor)
	dc.DrawStringAnchored(opts.Title, float64(width)/2, 24, 0.5, 0.5)
	if opts.Subtitle != "" {
		dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: 15}))
		setColor(dc, subtleColor)
		dc.DrawStringAnchored(opts.Subtitle, float64(width)/2, 48, 0.5, 0.5)
	}

	// Column header letters.
	dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: fontHeader}))
	setColor(dc, subtleColor)
	for c := 0; c < cols; c++ {
		cx := float64(gridPad) + float64(c)*cellSize + cellSize/2
		dc.DrawStringAnchored(string(headerLetter[c]), cx, titleH+headerH/2, 0.5, 0.5)
	}

	regular := truetype.NewFace(font, &truetype.Options{Size: fontRegular})
	top := float64(titleH + headerH)
	for idx, cell := range opts.Cells {
		r := idx / cols
		c := idx % cols
		x := float64(gridPad) + float64(c)*cellSize
		y := top + float64(r)*cellSize

		switch {
		case cell.Free:
			setColor(dc, freeColor)
		case cell.Marked:
			setColor(dc, markedColor)
		default:
			setColor(dc, cellColor)
		}
		dc.DrawRoundedRectangle(x+3, y+3, cellSize-6, cellSize-6, 10)
		dc.Fill()

		setColor(dc, gridLine)
		dc.SetLineWidth(2)
		dc.DrawRoundedRectangle(x+3, y+3, cellSize-6, cellSize-6, 10)
		dc.Stroke()

		dc.SetFontFace(regular)
		setColor(dc, textColor)
		dc.DrawStringWrapped(cell.Text, x+cellSize/2, y+cellSize/2, 0.5, 0.5,
			cellSize-20, 1.25, gg.AlignCenter)

		if cell.Marked && !cell.Free {
			drawCheck(dc, x+cellSize-24, y+20)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("render: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func drawCheck(dc *gg.Context, x, y float64) {
	setColor(dc, textColor)
	dc.SetLineWidth(3)
	dc.MoveTo(x-8, y)
	dc.LineTo(x-3, y+6)
	dc.LineTo(x+7, y-7)
	dc.Stroke()
}

func setColor(dc *gg.Context, c [3]float64) { dc.SetRGB(c[0], c[1], c[2]) }
