package game

import (
	"math/rand"
	"strings"
	"testing"
)

func TestHairpinFitsTheBoardAndVaries(t *testing.T) {
	hairpins, spirals := 0, 0
	for seed := int64(1); seed <= 60; seed++ {
		p := NewPathForLevel(1, rand.New(rand.NewSource(seed)))
		if p.Len < 11 {
			t.Fatalf("seed %d: path too short (%.2f m)", seed, p.Len)
		}
		limit := float32(1.31) // the spiral rim
		if strings.HasPrefix(p.Key, "hairpin") {
			limit = 1.24
		}
		for s := float32(0); s <= p.Len; s += 0.05 {
			if r := p.PointAt(s).Len(); r > limit {
				t.Fatalf("seed %d: path leaves the board at s=%.2f (r=%.3f)", seed, s, r)
			}
		}
		if strings.HasPrefix(p.Key, "hairpin") {
			hairpins++
		} else {
			spirals++
		}
	}
	if hairpins == 0 || spirals == 0 {
		t.Fatalf("layout families should both appear: hairpin=%d spiral=%d", hairpins, spirals)
	}
}

func TestHairpinTurnsAreTangentContinuous(t *testing.T) {
	p := NewHairpinPath(0.4, 7, 0.22)
	prev, prevD := p.At(0)
	for s := float32(0.02); s <= p.Len; s += 0.02 {
		pos, d := p.At(s)
		if step := pos.Sub(prev).Len(); step > 0.06 {
			t.Fatalf("path jumps %.3f m at s=%.2f", step, s)
		}
		if dot := d.Dot(prevD); dot < 0.96 { // arc chords turn a few degrees
			t.Fatalf("tangent kink at s=%.2f: dot=%.3f", s, dot)
		}
		prev, prevD = pos, d
	}
}

func TestShapedRoutesStayOnTheirBoard(t *testing.T) {
	boards := []Board{
		CircleBoard(3.10),
		{W: 3.5, H: 2.5, Oval: true},
		{W: 3.0, H: 2.2, Corner: 0.55},
		{W: 2.6, H: 2.6, Corner: 0.60},
		{W: 3.8, H: 1.6, Corner: 0.50},
		{W: 3.2, H: 2.4, Corner: 0.60},
		{W: 3.6, H: 2.0, Corner: 0.90},
	}
	routes := []struct {
		name string
		gen  func(b Board) *Path
	}{
		{"spiral", func(b Board) *Path { return NewSpiralOnBoard(b, 0.16, 4.0, 0.30) }},
		{"lane5", func(b Board) *Path { return NewLanePath(b, 0.15, 5, 0.26) }},
		{"lane8", func(b Board) *Path { return NewLanePath(b, 0.14, 8, 0.22) }},
		{"arc", func(b Board) *Path { return NewArcSwitchbackOnBoard(b, 0.14, 6, 0.24, 1.9) }},
	}
	for _, b := range boards {
		for _, r := range routes {
			p := r.gen(b)
			if p.Len < 6 {
				t.Fatalf("%s on %.1fx%.1f: path too short (%.1f m)", r.name, b.W, b.H, p.Len)
			}
			for s := float32(0); s <= p.Len; s += 0.02 {
				pt := p.PointAt(s)
				if w := b.WidthAt(pt.Y()); pt.X() > w+0.005 || pt.X() < -w-0.005 {
					t.Fatalf("%s on %.1fx%.1f leaves the felt at s=%.2f: x=%.3f w=%.3f",
						r.name, b.W, b.H, s, pt.X(), w)
				}
			}
		}
	}
}
