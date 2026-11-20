package game

import (
	"fmt"
	"math"
	"math/rand"
)

// ValidateRoute checks a route against its board: it must stay on the felt, keep
// separate laps at least one bead apart, and be long enough to play.
func ValidateRoute(b Board, p *Path) error {
	// A one-stroke outline is only its silhouette, so it is legitimately shorter
	// than the winding families; keep a floor that still gives a playable chain.
	if p == nil || p.Len < 7.2 {
		return fmt.Errorf("route too short (%.1f m)", p.Len)
	}
	const margin = 0.055
	for s := float32(0); s <= p.Len; s += 0.03 {
		pt := p.PointAt(s)
		if w := b.WidthAt(pt.Y()); math.Abs(float64(pt.X())) > float64(w-margin) {
			return fmt.Errorf("route leaves the felt at s=%.2f (x=%.2f w=%.2f)", s, pt.X(), w)
		}
	}
	// Separate laps must keep beads apart even where the route folds back.
	const step = 0.05
	type sample struct{ x, y, s float32 }
	var q []sample
	for s := float32(0); s <= p.Len; s += step {
		pt := p.PointAt(s)
		q = append(q, sample{pt.X(), pt.Y(), s})
	}
	for i := range q {
		for j := i + 1; j < len(q); j++ {
			if q[j].s-q[i].s < 0.55 {
				continue
			}
			d := math.Hypot(float64(q[i].x-q[j].x), float64(q[i].y-q[j].y))
			if d < 0.135 {
				return fmt.Errorf("laps collide at s=%.2f/%.2f (%.3f m)", q[i].s, q[j].s, d)
			}
		}
	}
	return nil
}

// grow scales a board up, used to give one-stroke outlines enough perimeter.
func grow(b Board, k float32) Board {
	b.W *= k
	b.H *= k
	b.Corner *= k
	return b
}

func randomBoard(rng *rand.Rand) Board {
	switch rng.Intn(4) {
	case 0:
		return CircleBoard(3.50 + rng.Float32()*0.30)
	case 1:
		return Board{W: 4.00 + rng.Float32()*0.40, H: 2.70 + rng.Float32()*0.40, Oval: true}
	case 2:
		return Board{W: 3.50 + rng.Float32()*0.45, H: 2.50 + rng.Float32()*0.45, Corner: 0.60 + rng.Float32()*0.20}
	default:
		return Board{W: 4.20 + rng.Float32()*0.40, H: 2.10 + rng.Float32()*0.35, Corner: 0.55 + rng.Float32()*0.15}
	}
}

func randomShape(rng *rand.Rand) RingShape {
	switch rng.Intn(3) {
	case 0:
		return RingShape{Square: 0.55 + rng.Float32()*0.3, Rot: rng.Float32() * math.Pi}
	case 1:
		return RingShape{Lobes: 3 + rng.Intn(3), Amp: 0.12 + rng.Float32()*0.08, Rot: rng.Float32() * math.Pi}
	default:
		return RingShape{}
	}
}

// randomSingleLine picks one silhouette for the single-line family: the route is
// always "one line winding inwards", only the shape it draws changes.
func randomSingleLine(b Board, rng *rand.Rand) (Board, *Path) {
	const inset float32 = 0.18
	// One-stroke outlines are only their silhouette, so they get a bigger board
	// to reach a comparable chain length.
	doodleBoard := func() Board { return grow(b, 1.38) }
	switch rng.Intn(9) {
	case 0:
		return b, NewPetalSpiralOnBoard(b, inset, 3.0+rng.Float32()*0.6, 0.26+rng.Float32()*0.08,
			4+float32(rng.Intn(3)), 0.045+rng.Float32()*0.025)
	case 1:
		return b, NewSnailSpiralOnBoard(b, inset, 2.8+rng.Float32()*0.5, 0.32+rng.Float32()*0.06, 1.3+rng.Float32()*0.2)
	case 2:
		p := NewEccentricSpiralOnBoard(b, inset, 3.4+rng.Float32()*0.8, 0.22+rng.Float32()*0.08,
			0.10+rng.Float32()*0.08, rng.Float32()*2*math.Pi)
		if rng.Intn(2) == 1 {
			p.Reverse()
		}
		return b, p
	case 3:
		return b, NewPitchSpiralOnBoard(b, inset, 3.0+rng.Float32()*0.6, 0.24+rng.Float32()*0.06, 1.25+rng.Float32()*0.3)
	case 4:
		bb := doodleBoard()
		return bb, NewDoodleOutline(bb, inset, "heart", heartOutline(), 2)
	case 5:
		bb := doodleBoard()
		return bb, NewDoodleOutline(bb, inset, "apple", appleOutline(), 2)
	case 6:
		bb := doodleBoard()
		return bb, NewDoodleOutline(bb, inset, "penta", pentagonOutline(), 1)
	case 7:
		bb := doodleBoard()
		return bb, NewDoodleOutline(bb, inset, "eye", eyeOutline(), 2)
	default:
		bb := doodleBoard()
		return bb, NewDoodleOutline(bb, inset, "cup", cupOutline(), 1)
	}
}

// randomRoute builds one family with fresh parameters. The caller validates it.
func randomRoute(b Board, rng *rand.Rand) (Board, *Path) {
	const inset float32 = 0.16
	// Weighted family choice: the families that almost always pass the guard
	// (arc switchback, maze) used to dominate because failing rolls re-roll.
	weights := []int{40, 12, 14, 12, 12, 10} // single-line, layers, maze, wave, square, arc
	total := 0
	for _, w := range weights {
		total += w
	}
	pick := rng.Intn(total)
	family := 0
	for i, w := range weights {
		if pick < w {
			family = i
			break
		}
		pick -= w
	}
	switch family {
	case 0: // single line winding inwards, silhouette random
		return randomSingleLine(b, rng)
	case 1: // layered rings, never more than two layers
		shapes := []RingShape{{}, randomShape(rng)}
		return b, NewLayeredRingsOnBoard(b, 0.18, shapes, []float32{1.2 + rng.Float32()*0.5})
	case 2: // maze switchback
		p := NewMazePath(b, 0.14, 4+rng.Intn(3), 0.26+rng.Float32()*0.06, 0.10+rng.Float32()*0.04)
		if rng.Intn(2) == 1 {
			p.Reverse()
		}
		return b, p
	case 3: // wavy switchback
		humps := 2
		if rng.Intn(2) == 1 {
			humps = 4
		}
		p := NewWavyLanePath(b, 0.14, 4+rng.Intn(3), 0.24+rng.Float32()*0.06,
			0.14+rng.Float32()*0.10, humps)
		if rng.Intn(2) == 1 {
			p.Reverse()
		}
		return b, p
	case 4: // square spiral
		p := NewSquareSpiralOnBoard(b, 0.15, 0.28+rng.Float32()*0.08, 0.13+rng.Float32()*0.03)
		if rng.Intn(2) == 1 {
			p.Reverse()
		}
		return b, p
	default: // arc switchback
		p := NewArcSwitchbackOnBoard(b, 0.14, 5+rng.Intn(3), 0.24+rng.Float32()*0.08, 1.8+rng.Float32()*0.3)
		if rng.Intn(2) == 1 {
			p.Reverse()
		}
		return b, p
	}
}

// NewLayoutForLevel picks the board shape and the route family for a level. Bad
// rolls (folds too close, runs off the felt) are rejected and re-rolled.
func NewLayoutForLevel(level int, rng *rand.Rand) (Board, *Path) {
	for i := 0; i < 60; i++ {
		b := randomBoard(rng)
		rb, p := randomRoute(b, rng)
		// Round and oval boards are rotationally symmetric, so the whole route
		// can be spun for extra variety; a rounded rectangle cannot.
		if rb.Oval {
			p.Rotate(rng.Float32() * 2 * math.Pi)
		}
		if ValidateRoute(rb, p) == nil {
			return rb, p
		}
	}
	b := CircleBoard(3.10)
	return b, NewPetalSpiralOnBoard(b, 0.16, 3.4, 0.30, 5, 0.06)
}
