package vr

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// TestMat34Layout checks OpenVR's row-major 3x4 becomes the column-major 4x4
// mgl32 expects, translation included.
func TestMat34Layout(t *testing.T) {
	raw := [12]float32{
		1, 0, 0, 1,
		0, 1, 0, 2,
		0, 0, 1, 3,
	}
	m := mat34(raw)
	got := m.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	want := mgl32.Vec4{1, 2, 3, 1}
	if got != want {
		t.Fatalf("origin mapped to %v, want %v", got, want)
	}
	if m != mgl32.Translate3D(1, 2, 3) {
		t.Fatalf("matrix %v, want translation (1,2,3)", m)
	}
}

// TestTranspose4 pins the row-major to column-major conversion used for the
// projection matrix.
func TestTranspose4(t *testing.T) {
	raw := [16]float32{
		1, 2, 3, 4,
		5, 6, 7, 8,
		9, 10, 11, 12,
		13, 14, 15, 16,
	}
	m := transpose4(raw)
	want := mgl32.Mat4{
		1, 5, 9, 13,
		2, 6, 10, 14,
		3, 7, 11, 15,
		4, 8, 12, 16,
	}
	if m != want {
		t.Fatalf("transpose %v, want %v", m, want)
	}
}

// TestButtonMasks documents openvr.h's ButtonMaskFromId: the mask for button id
// n is 1<<n.
func TestButtonMasks(t *testing.T) {
	cases := []struct {
		name string
		id   uint
		mask uint64
	}{
		{"ApplicationMenu", 1, buttonMenu},
		{"Grip", 2, buttonGrip},
		{"A", 7, buttonA},
		{"SteamVR_Touchpad", 32, buttonStick},
		{"SteamVR_Trigger", 33, buttonTrigger},
	}
	for _, c := range cases {
		if c.mask != 1<<c.id {
			t.Errorf("%s: mask %#x, want %#x", c.name, c.mask, uint64(1)<<c.id)
		}
	}
}
