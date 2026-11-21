package main

import (
	"image"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// TestPixRGBACopiesRowsVerbatim guards the fast path the controller loader
// takes for decoded PNGs: a padded stride must not shift the picture.
func TestPixRGBACopiesRowsVerbatim(t *testing.T) {
	w, h, stride := 3, 2, 3*4+8 // padded rows, as image.NRGBA allows
	src := &image.NRGBA{Pix: make([]byte, stride*h), Stride: stride, Rect: image.Rect(0, 0, w, h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*stride + x*4
			src.Pix[i+0] = byte(x*40 + 1)
			src.Pix[i+1] = byte(y*40 + 2)
			src.Pix[i+2] = 3
			src.Pix[i+3] = 4
		}
	}
	got := pixRGBA(src)
	if got.w != w || got.h != h || len(got.pix) != w*h*4 {
		t.Fatalf("bad shape %dx%d, %d bytes", got.w, got.h, len(got.pix))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s := src.Pix[y*stride+x*4:]
			d := got.pix[(y*w+x)*4:]
			for c := 0; c < 4; c++ {
				if d[c] != s[c] {
					t.Fatalf("pixel (%d,%d) channel %d = %d, want %d", x, y, c, d[c], s[c])
				}
			}
		}
	}
}

// TestRayPlaneRoundTrip pins the desktop aiming pipe: a board point projected
// to screen and unprojected again must land on the same spot.
func TestRayPlaneRoundTrip(t *testing.T) {
	cam := camera{yaw: 0.2, pitch: 0.7, dist: 4.6}
	vp, _ := cameraVP(cam, 1600, 900)
	inv := vp.Inv()
	for _, want := range []mgl32.Vec3{{0, 0.02, 0}, {0.8, 0.02, -0.9}, {-1.1, 0.02, 0.6}} {
		clip := vp.Mul4x1(mgl32.Vec4{want[0], want[1], want[2], 1})
		ndcX, ndcY := clip[0]/clip[3], clip[1]/clip[3]
		got, ok := rayPlane(inv, ndcX, ndcY, 0.02)
		if !ok {
			t.Fatalf("ray missed the plane for %v", want)
		}
		if got.Sub(want).Len() > 1e-3 {
			t.Fatalf("unprojected %v, want %v", got, want)
		}
	}
}
