package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

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
