// Package game holds the Zuma rules: a chain of coloured balls rolls along a
// spiral towards a hole, the frog in the middle shoots balls into it, and
// runs of three or more pop. Nothing here touches OpenGL, so the rules can be
// tested on their own.
package game

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/go-gl/mathgl/mgl32"
)

// Path is an arc-length parameterised polyline on the board plane (XZ). s=0 is
// where balls enter, s=Len is the mouth of the hole.
type Path struct {
	pts []mgl32.Vec2
	cum []float32
	Len float32

	// Key identifies a layout so the renderer can tell when the board geometry
	// has to be rebuilt for a new level.
	Key string

	// Generation parameters, so the renderer can tell when the board geometry
	// has to be rebuilt for a new level.
	Start    float32
	Turns    float32
	ROuter   float32
	RInner   float32
	Reversed bool
}

// NewSpiralPath builds the board's single spiral: it starts at the outer rim
// at startAngle and winds inward, losing radius, until it reaches the hole.
func NewSpiralPath(startAngle, turns, rOuter, rInner float32) *Path {
	const samples = 2400
	pts := make([]mgl32.Vec2, 0, samples+2)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := startAngle + t*turns*2*math.Pi
		r := rOuter + (rInner-rOuter)*t
		pts = append(pts, mgl32.Vec2{r * float32(math.Cos(float64(a))), r * float32(math.Sin(float64(a)))})
	}
	p := NewPath(pts)
	p.Start, p.Turns, p.ROuter, p.RInner = startAngle, turns, rOuter, rInner
	p.Key = fmt.Sprintf("spiral %.3f %.3f %.3f %.3f", startAngle, turns, rOuter, rInner)
	return p
}

// NewPath measures the cumulative arc length of a polyline.
func NewPath(pts []mgl32.Vec2) *Path {
	p := &Path{pts: append([]mgl32.Vec2(nil), pts...), cum: make([]float32, len(pts))}
	for i := 1; i < len(pts); i++ {
		p.cum[i] = p.cum[i-1] + pts[i].Sub(pts[i-1]).Len()
	}
	if n := len(p.cum); n > 0 {
		p.Len = p.cum[n-1]
	}
	return p
}

// At returns the point and unit tangent at arc length s. s is clamped to the
// ends of the path.
func (p *Path) At(s float32) (mgl32.Vec2, mgl32.Vec2) {
	n := len(p.pts)
	if n == 0 {
		return mgl32.Vec2{}, mgl32.Vec2{0, 1}
	}
	if s <= 0 {
		d := p.pts[1].Sub(p.pts[0])
		if l := d.Len(); l > 0 {
			d = d.Mul(1 / l)
		}
		return p.pts[0], d
	}
	if s >= p.Len {
		d := p.pts[n-1].Sub(p.pts[n-2])
		if l := d.Len(); l > 0 {
			d = d.Mul(1 / l)
		}
		return p.pts[n-1], d
	}
	i := sort.Search(n, func(i int) bool { return p.cum[i] >= s })
	if i == 0 {
		i = 1
	}
	seg := p.cum[i] - p.cum[i-1]
	var t float32
	if seg > 0 {
		t = (s - p.cum[i-1]) / seg
	}
	pos := p.pts[i-1].Add(p.pts[i].Sub(p.pts[i-1]).Mul(t))
	d := p.pts[i].Sub(p.pts[i-1])
	if l := d.Len(); l > 0 {
		d = d.Mul(1 / l)
	}
	return pos, d
}

// PointAt is At without the tangent.
func (p *Path) PointAt(s float32) mgl32.Vec2 {
	pos, _ := p.At(s)
	return pos
}

// BoardRadius is the distance from the board centre to the rim.
const BoardRadius float32 = 1.55

// Turret is the frog's seat: the centre of the board.
var Turret = mgl32.Vec2{0, 0}

// Spiral geometry shared by the logic and the renderer.
const (
	spiralStart = 3.90
	spiralTurns = 1.90
	spiralOuter = 1.30
	spiralInner = 0.52
)

// NewPathForBoard is the fixed spiral used by tests and as a stand-in before a
// level exists.
func NewPathForBoard() *Path {
	return NewSpiralPath(spiralStart, spiralTurns, spiralOuter, spiralInner)
}

// Rotate spins the whole route about the board centre. The renderer keys on
// Path.Key, so the angle is folded into it.
func (p *Path) Rotate(a float32) {
	c, sn := float32(math.Cos(float64(a))), float32(math.Sin(float64(a)))
	for i := range p.pts {
		x, z := p.pts[i].X(), p.pts[i].Y()
		p.pts[i] = mgl32.Vec2{x*c - z*sn, x*sn + z*c}
	}
	p.Key += fmt.Sprintf("|rot%.2f", a)
}

// Reverse flips the direction of travel: the spawn becomes the far end and the
// hole becomes the other one.
func (p *Path) Reverse() {
	for i, j := 0, len(p.pts)-1; i < j; i, j = i+1, j-1 {
		p.pts[i], p.pts[j] = p.pts[j], p.pts[i]
	}
	p.cum[0] = 0
	for i := 1; i < len(p.pts); i++ {
		p.cum[i] = p.cum[i-1] + p.pts[i].Sub(p.pts[i-1]).Len()
	}
	p.Len = p.cum[len(p.cum)-1]
	p.Reversed = !p.Reversed
	p.Key += "|r"
}

// NewPathForLevel builds a fresh spiral for a level: roughly twice the old
// length, with a random rotation and a random inner end so the hole is not
// always in the same place (and not necessarily central).
func NewPathForLevel(level int, rng *rand.Rand) *Path {
	if rng.Intn(2) == 1 {
		lanes := 5 + rng.Intn(6)             // 5..10 switchback lanes
		spacing := 0.19 + rng.Float32()*0.09 // 0.19..0.28 between lanes
		p := NewHairpinPath(rng.Float32()*2*math.Pi, lanes, spacing)
		if rng.Intn(2) == 1 {
			p.Reverse()
		}
		return p
	}
	start := rng.Float32() * 2 * math.Pi
	inner := 0.30 + rng.Float32()*0.55 // hole anywhere from the middle to the outer half
	pitch := 0.19 + rng.Float32()*0.07 // keeps neighbouring turns apart
	turns := (spiralOuter - inner) / pitch
	if turns < 2.2 { // keep the spawn and the hole visibly apart
		turns = 2.2
	}
	if turns > 5.0 {
		turns = 5.0
	}
	p := NewSpiralPath(start, turns, spiralOuter, inner)
	if rng.Intn(2) == 1 {
		// Half the rounds feed from the middle and the hole sits out near the
		// rim, so the chain rolls towards the player instead of inwards.
		p.Reverse()
	}
	return p
}

// NewHairpinPath builds a switchback route: parallel lanes joined by 180 degree
// turns, so the chain doubles back across the board instead of spiralling. The
// lane ends are tangent to the turns, so there are no corners to jam in.
func NewHairpinPath(rot float32, lanes int, spacing float32) *Path {
	const rMax = 1.22
	if lanes < 3 {
		lanes = 3
	}
	h := float32(lanes) * spacing
	half := (h - spacing) / 2
	w := float32(math.Sqrt(float64(rMax*rMax-half*half))) - spacing/2

	var pts []mgl32.Vec2
	z0 := -half
	pts = append(pts, mgl32.Vec2{-w, z0})
	arc := func(cx, cz, a0, a1 float32) {
		const steps = 44
		for k := 1; k <= steps; k++ {
			a := a0 + (a1-a0)*float32(k)/steps
			pts = append(pts, mgl32.Vec2{
				cx + spacing/2*float32(math.Cos(float64(a))),
				cz + spacing/2*float32(math.Sin(float64(a))),
			})
		}
	}
	for i := 0; i < lanes; i++ {
		z := z0 + float32(i)*spacing
		if i%2 == 0 {
			pts = append(pts, mgl32.Vec2{w, z})
			if i < lanes-1 {
				arc(w, z+spacing/2, -math.Pi/2, math.Pi/2)
			}
		} else {
			pts = append(pts, mgl32.Vec2{-w, z})
			if i < lanes-1 {
				arc(-w, z+spacing/2, -math.Pi/2, -3*math.Pi/2)
			}
		}
	}
	c, s := math.Cos(float64(rot)), math.Sin(float64(rot))
	for i := range pts {
		x, z := float64(pts[i].X()), float64(pts[i].Y())
		pts[i] = mgl32.Vec2{float32(c*x - s*z), float32(s*x + c*z)}
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("hairpin %d %.3f %.3f", lanes, spacing, rot)
	return p
}

// NewArcSwitchbackPath builds concentric arc lanes joined by radial U-turns.
// It hugs the rim instead of filling a rectangle, and every joint is tangent
// continuous.
func NewArcSwitchbackPath(rot float32, lanes int, spacing, halfAngle float32) *Path {
	const rMax = 1.22
	if lanes < 3 {
		lanes = 3
	}
	if maxR := rMax - spacing/2; float32(lanes-1)*spacing > maxR-0.10 {
		lanes = int((maxR-0.10)/spacing) + 1
	}
	r0 := rMax - spacing/2

	var pts []mgl32.Vec2
	pt := func(r, a float32) mgl32.Vec2 {
		return mgl32.Vec2{r * float32(math.Cos(float64(a))), r * float32(math.Sin(float64(a)))}
	}
	arc := func(r, from, to float32, steps int) {
		for k := 1; k <= steps; k++ {
			pts = append(pts, pt(r, from+(to-from)*float32(k)/float32(steps)))
		}
	}
	turn := func(r, a, sign float32) {
		// Semicircle from radius r to r-spacing at angle a, bulging sideways.
		c := pt(r-spacing/2, a)
		base := a + sign*float32(math.Pi)/2
		const steps = 26
		for k := 1; k <= steps; k++ {
			p := base - sign*float32(math.Pi)*float32(k)/steps
			pts = append(pts, c.Add(mgl32.Vec2{
				spacing / 2 * float32(math.Cos(float64(p))),
				spacing / 2 * float32(math.Sin(float64(p))),
			}))
		}
	}

	for i := 0; i < lanes; i++ {
		r := r0 - float32(i)*spacing
		if i == 0 {
			pts = append(pts, pt(r, -halfAngle))
		}
		if i%2 == 0 {
			arc(r, -halfAngle, halfAngle, 40)
			if i < lanes-1 {
				turn(r, halfAngle, 1)
			}
		} else {
			arc(r, halfAngle, -halfAngle, 40)
			if i < lanes-1 {
				turn(r, -halfAngle, -1)
			}
		}
	}
	c, s2 := math.Cos(float64(rot)), math.Sin(float64(rot))
	for i := range pts {
		x, z := float64(pts[i].X()), float64(pts[i].Y())
		pts[i] = mgl32.Vec2{float32(c*x - s2*z), float32(s2*x + c*z)}
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("arcswitch %d %.3f %.3f %.3f", lanes, spacing, halfAngle, rot)
	return p
}
