// Package game holds the Zuma rules: a chain of coloured balls rolls along a
// spiral towards a hole, the frog in the middle shoots balls into it, and
// runs of three or more pop. Nothing here touches OpenGL, so the rules can be
// tested on their own.
package game

import (
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

	// Generation parameters, so the renderer can tell when the board geometry
	// has to be rebuilt for a new level.
	Start  float32
	Turns  float32
	ROuter float32
	RInner float32
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

// NewPathForLevel builds a fresh spiral for a level: roughly twice the old
// length, with a random rotation and a random inner end so the hole is not
// always in the same place (and not necessarily central).
func NewPathForLevel(level int, rng *rand.Rand) *Path {
	start := rng.Float32() * 2 * math.Pi
	inner := 0.30 + rng.Float32()*0.20 // 0.30..0.50: keeps the turns apart on the board
	return NewSpiralPath(start, 3.8, spiralOuter, inner)
}
