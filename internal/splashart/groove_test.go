package splashart

import (
	"image/png"
	"os"
	"testing"
)

// TestGrooveMatchesSpiral samples the painted PNG along the spiral the renderer
// animates beads on. Every sample has to land on the dark channel (or one of its
// lips); a bright felt pixel means the beads would float beside the groove.
func TestGrooveMatchesSpiral(t *testing.T) {
	f, err := os.Open("../../internal/render/splash_bg.png")
	if err != nil {
		t.Skip("splash art has not been generated yet")
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	dark := 0
	total := 0
	for i := 0; i <= 240; i++ {
		s := SpiralLength * float32(i) / 240
		x, y, ok := SpiralAt(s)
		if !ok {
			t.Fatalf("no point at s=%.1f", s)
		}
		// SpiralAt works in image rows; the PNG stores them the same way.
		c := img.At(int(x), int(y))
		r, g, b, _ := c.RGBA()
		if r+g+b < 3*40*257/2 { // channel is near-black; felt is teal
			dark++
		}
		total++
	}
	if dark*10 < total*9 {
		t.Fatalf("only %d/%d spiral samples fall in the painted groove", dark, total)
	}
}
