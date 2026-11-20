package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"zuma/internal/splashart"
)

// TestSplashFitKeepsThePosterCentred pins the cover mapping: the art centre
// lands in the middle of the framebuffer and the corners stay in view.
func TestSplashFitKeepsThePosterCentred(t *testing.T) {
	for _, size := range [][2]int{{1600, 900}, {1728, 1728}, {1280, 800}} {
		w, h := size[0], size[1]
		vp := SplashFit(w, h)
		c := vp.Mul4x1(mgl32.Vec4{splashart.ArtW / 2, splashart.ArtH / 2, 0, 1})
		if math.Abs(float64(c[0])) > 1e-4 || math.Abs(float64(c[1])) > 1e-4 {
			t.Fatalf("%dx%d: centre maps to (%.4f,%.4f), want (0,0)", w, h, c[0], c[1])
		}
		// Every corner of the poster must stay inside the frame, and one axis
		// must be filled exactly so the poster is as large as it can be.
		filledX, filledY := false, false
		for _, corner := range [][2]float32{{0, 0}, {splashart.ArtW, 0}, {0, splashart.ArtH}, {splashart.ArtW, splashart.ArtH}} {
			p := vp.Mul4x1(mgl32.Vec4{corner[0], corner[1], 0, 1})
			if math.Abs(float64(p[0])) > 1.0001 || math.Abs(float64(p[1])) > 1.0001 {
				t.Fatalf("%dx%d: corner %v maps to (%.3f,%.3f), outside the frame", w, h, corner, p[0], p[1])
			}
			if math.Abs(math.Abs(float64(p[0]))-1) < 1e-3 {
				filledX = true
			}
			if math.Abs(math.Abs(float64(p[1]))-1) < 1e-3 {
				filledY = true
			}
		}
		if !filledX && !filledY {
			t.Fatalf("%dx%d: poster does not fill either axis", w, h)
		}
	}
}

// TestSplashPanelSitsInFrontOfTheHead checks the head-locked screen is 2.6 m
// ahead, centred, and the right way up.
func TestSplashPanelSitsInFrontOfTheHead(t *testing.T) {
	panel := SplashPanel(mgl32.Ident4())
	mid := panel.Mul4x1(mgl32.Vec4{splashart.ArtW / 2, splashart.ArtH / 2, 0, 1})
	if math.Abs(float64(mid[0])) > 1e-4 || math.Abs(float64(mid[1])) > 1e-4 {
		t.Fatalf("poster centre is off-axis: (%.3f,%.3f)", mid[0], mid[1])
	}
	if math.Abs(float64(mid[2])+2.6) > 1e-3 {
		t.Fatalf("poster sits at z=%.3f, want -2.6", mid[2])
	}
	top := panel.Mul4x1(mgl32.Vec4{splashart.ArtW / 2, splashart.ArtH, 0, 1})
	if top[1] <= mid[1] {
		t.Fatalf("art y grows downwards on screen: top=%.3f mid=%.3f", top[1], mid[1])
	}
}
