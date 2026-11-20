package splashart

import (
	"math"
	"testing"
)

// TestSpiralStaysInsideTheGroove checks the animated path matches the painted
// channel: it starts at the outer radius, ends at the inner one, and never
// wanders outside the felt.
func TestSpiralStaysInsideTheGroove(t *testing.T) {
	if SpiralLength < 1500 {
		t.Fatalf("spiral is too short for a chain: %.0f px", SpiralLength)
	}
	firstX, firstY, ok := SpiralAt(0)
	if !ok {
		t.Fatal("no point at the outer end")
	}
	if d := radius(firstX, firstY); math.Abs(d-SpiralOuter) > 3 {
		t.Fatalf("outer end sits at radius %.1f, want %.0f", d, SpiralOuter)
	}
	lastX, lastY, ok := SpiralAt(SpiralLength)
	if !ok {
		t.Fatal("no point at the inner end")
	}
	if d := radius(lastX, lastY); math.Abs(d-SpiralInner) > 3 {
		t.Fatalf("inner end sits at radius %.1f, want %.0f", d, SpiralInner)
	}
	if _, _, ok := SpiralAt(SpiralLength + 1); ok {
		t.Fatal("a point past the end would let beads float outside the groove")
	}
	for i := 0; i <= 64; i++ {
		s := SpiralLength * float32(i) / 64
		x, y, ok := SpiralAt(s)
		if !ok {
			t.Fatalf("no point at s=%.1f", s)
		}
		if d := radius(x, y); d > SpiralOuter+2 || d < SpiralInner-2 {
			t.Fatalf("s=%.1f leaves the felt: radius %.1f", s, d)
		}
	}
}

// TestSpiralArcLengthIsMonotonic guards the binary search inside SpiralAt.
func TestSpiralArcLengthIsMonotonic(t *testing.T) {
	prev := float32(0)
	for i := 0; i <= 200; i++ {
		s := SpiralLength * float32(i) / 200
		x, y, ok := SpiralAt(s)
		if !ok {
			t.Fatalf("no point at s=%.1f", s)
		}
		if i == 0 {
			continue
		}
		px, py, _ := SpiralAt(prev)
		if math.Hypot(float64(x-px), float64(y-py)) < 0.5 {
			t.Fatalf("samples at s=%.1f and s=%.1f barely move", prev, s)
		}
		prev = s
	}
}

func radius(x, y float32) float64 {
	return math.Hypot(float64(x-CenterX), float64(y-CenterY))
}
