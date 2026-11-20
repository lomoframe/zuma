// Package font rasterises sign text from a bundled glyph subset with
// golang.org/x/image, so the world can show real Chinese sign text without a
// system font and without any C rasteriser.
package font

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Line is a rasterised line of text: an 8-bit coverage mask, row major.
type Line struct {
	W, H int
	Mask []byte
}

// Render rasterises one line of UTF-8 text from font file data at the given
// pixel height.
func Render(faceData []byte, text string, px int) (Line, error) {
	f, err := opentype.Parse(faceData)
	if err != nil {
		return Line{}, fmt.Errorf("parse font: %w", err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(px),
		DPI:     72,
		Hinting: xfont.HintingNone,
	})
	if err != nil {
		return Line{}, fmt.Errorf("open face: %w", err)
	}

	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	if ascent <= 0 {
		ascent = px
	}
	if descent < 0 {
		descent = 0
	}

	width := xfont.MeasureString(face, text).Ceil()
	height := ascent + descent
	if width <= 0 || height <= 0 {
		return Line{}, errors.New("empty text")
	}

	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	drawer := xfont.Drawer{
		Dst:  mask,
		Src:  image.NewUniform(color.Alpha{A: 0xff}),
		Face: face,
		Dot:  fixed.P(0, ascent),
	}
	drawer.DrawString(text)
	return Line{W: width, H: height, Mask: mask.Pix}, nil
}

// RenderFile rasterises text with the font file at path.
func RenderFile(path, text string, px int) (Line, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Line{}, err
	}
	return Render(data, text, px)
}
