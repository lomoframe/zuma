package game

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// Board is the felt area a route has to fit into. Routes are laid out in board
// space (X across, Z towards the player), so any route generator only needs
// WidthAt to know how much room it has at a given height.
type Board struct {
	W, H   float32 // felt size in metres
	Corner float32 // rounded rectangle corner radius; 0 means square corners
	Oval   bool
}

// CircleBoard is the classic round felt.
func CircleBoard(d float32) Board { return Board{W: d, H: d, Oval: true} }

func (b Board) HalfW() float32 { return b.W / 2 }
func (b Board) HalfH() float32 { return b.H / 2 }

// WidthAt is the half width of the felt at local height z (0 outside the felt).
func (b Board) WidthAt(z float32) float32 {
	hz := b.HalfH()
	if z <= -hz || z >= hz {
		return 0
	}
	if b.Oval {
		q := 1 - (z/hz)*(z/hz)
		if q <= 0 {
			return 0
		}
		return b.HalfW() * float32(math.Sqrt(float64(q)))
	}
	r := b.Corner
	if r <= 0 {
		return b.HalfW()
	}
	if r > hz {
		r = hz
	}
	inner := hz - r
	d := float32(0)
	if z > inner {
		d = z - inner
	} else if z < -inner {
		d = -inner - z
	}
	if d == 0 {
		return b.HalfW()
	}
	return b.HalfW() - r + float32(math.Sqrt(float64(r*r-d*d)))
}

// Inflate grows the board outwards, used for the wooden rim around the felt.
func (b Board) Inflate(d float32) Board {
	nb := b
	nb.W += 2 * d
	nb.H += 2 * d
	nb.Corner += d
	return nb
}

// Outline samples the felt edge as a closed counter-clockwise loop.
func (b Board) Outline(n int) []mgl32.Vec2 {
	if n < 16 {
		n = 16
	}
	pts := make([]mgl32.Vec2, 0, 2*n+2)
	half := b.HalfH()
	for i := 0; i <= n; i++ {
		z := -half + 2*half*float32(i)/float32(n)
		if z == -half {
			z += 0.0005
		}
		if z == half {
			z -= 0.0005
		}
		if w := b.WidthAt(z); w > 0 {
			pts = append(pts, mgl32.Vec2{w, z})
		}
	}
	for i := n; i >= 0; i-- {
		z := -half + 2*half*float32(i)/float32(n)
		if z == -half {
			z += 0.0005
		}
		if z == half {
			z -= 0.0005
		}
		if w := b.WidthAt(z); w > 0 {
			pts = append(pts, mgl32.Vec2{-w, z})
		}
	}
	return pts
}

// NewSpiralOnBoard winds a spiral inside the felt, squashed to its shape.
func NewSpiralOnBoard(b Board, inset, turns, inner float32) *Path {
	const samples = 2200
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	pts := make([]mgl32.Vec2, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := t * turns * 2 * math.Pi
		r := 1 + (inner-1)*t
		pts = append(pts, mgl32.Vec2{
			r * rx * float32(math.Cos(float64(a))),
			r * rz * float32(math.Sin(float64(a))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("spiralboard %.2fx%.2f %.2f %.2f %.2f %.2f", b.W, b.H, b.Corner, inset, turns, inner)
	return p
}

// NewLanePath lays parallel lanes inside the felt, each as wide as the board
// allows at its height, joined by 180 degree turns.
func NewLanePath(b Board, inset float32, lanes int, spacing float32) *Path {
	if lanes < 3 {
		lanes = 3
	}
	h := float32(lanes-1) * spacing
	maxHalf := b.HalfH() - inset - spacing/2
	if h/2 > maxHalf {
		h = 2 * maxHalf
		lanes = int(h/spacing) + 1
		if lanes < 3 {
			lanes = 3
		}
		h = float32(lanes-1) * spacing
	}
	z0 := -h / 2
	halfAt := func(z float32) float32 {
		w := b.WidthAt(z) - inset - spacing/2
		if w < 0.12 {
			w = 0.12
		}
		return w
	}

	var pts []mgl32.Vec2
	w0 := halfAt(z0)
	pts = append(pts, mgl32.Vec2{-w0, z0})
	for i := 0; i < lanes; i++ {
		z := z0 + float32(i)*spacing
		w := halfAt(z)
		dir := float32(1)
		if i%2 == 1 {
			dir = -1
		}
		pts = append(pts, mgl32.Vec2{dir * w, z})
		if i == lanes-1 {
			break
		}
		nz := z0 + float32(i+1)*spacing
		nw := halfAt(nz)
		// The turn leaves along dir and arrives along -dir, landing on the same
		// side the next lane starts from.
		dw := abs32(nw - w)
		if dw < 0.08 {
			cz := z + spacing/2
			const steps = 30
			for k := 1; k <= steps; k++ {
				a := -float32(math.Pi)/2 + float32(math.Pi)*float32(k)/steps
				pts = append(pts, mgl32.Vec2{
					dir * (w + spacing/2*float32(math.Cos(float64(a)))),
					cz + spacing/2*float32(math.Sin(float64(a))),
				})
			}
			continue
		}
		k := 0.8*spacing + 0.6*dw
		p1 := mgl32.Vec2{dir*w + dir*k, z}
		p2 := mgl32.Vec2{dir*nw + dir*k, nz}
		from := mgl32.Vec2{dir * w, z}
		to := mgl32.Vec2{dir * nw, nz}
		const steps = 26
		for j := 1; j <= steps; j++ {
			t := float32(j) / steps
			u := 1 - t
			pts = append(pts, from.Mul(u*u*u).
				Add(p1.Mul(3*u*u*t)).
				Add(p2.Mul(3*u*t*t)).
				Add(to.Mul(t*t*t)))
		}
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("laneboard %.2fx%.2f %.2f %d %.2f", b.W, b.H, b.Corner, lanes, spacing)
	return p
}

// NewArcSwitchbackOnBoard builds concentric lanes joined by radial U-turns,
// squashed to the felt shape.
func NewArcSwitchbackOnBoard(b Board, inset float32, lanes int, spacing, halfAngle float32) *Path {
	if lanes < 3 {
		lanes = 3
	}
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	unitSpacing := spacing / min32(rx, rz)
	r0 := 1 - unitSpacing/2
	if float32(lanes-1)*unitSpacing > r0-0.08 {
		lanes = int((r0-0.08)/unitSpacing) + 1
		if lanes < 3 {
			lanes = 3
		}
	}

	var pts []mgl32.Vec2
	pt := func(r, a float32) mgl32.Vec2 {
		return mgl32.Vec2{r * rx * float32(math.Cos(float64(a))), r * rz * float32(math.Sin(float64(a)))}
	}
	arc := func(r, from, to float32) {
		const steps = 34
		for k := 1; k <= steps; k++ {
			pts = append(pts, pt(r, from+(to-from)*float32(k)/steps))
		}
	}
	turn := func(r, a, sign float32) {
		// Cubic linking the end of this lane to the start of the next one. The
		// handles follow the arcs' tangents, so the joint has no kink and the
		// turn's radius stays generous instead of pinching down to half the
		// lane spacing.
		ca, sa := float32(math.Cos(float64(a))), float32(math.Sin(float64(a)))
		tx, ty := -sa*sign, ca*sign
		k := 1.7 * unitSpacing
		ax, ay := r*ca, r*sa
		bx, by := (r-unitSpacing)*ca, (r-unitSpacing)*sa
		p1x, p1y := ax+tx*k, ay+ty*k
		p2x, p2y := bx+tx*k, by+ty*k
		const steps = 30
		for j := 1; j <= steps; j++ {
			t := float32(j) / steps
			u := 1 - t
			x := u*u*u*ax + 3*u*u*t*p1x + 3*u*t*t*p2x + t*t*t*bx
			y := u*u*u*ay + 3*u*u*t*p1y + 3*u*t*t*p2y + t*t*t*by
			pts = append(pts, mgl32.Vec2{x * rx, y * rz})
		}
	}

	for i := 0; i < lanes; i++ {
		r := r0 - float32(i)*unitSpacing
		if i == 0 {
			pts = append(pts, pt(r, -halfAngle))
		}
		if i%2 == 0 {
			arc(r, -halfAngle, halfAngle)
			if i < lanes-1 {
				turn(r, halfAngle, 1)
			}
		} else {
			arc(r, halfAngle, -halfAngle)
			if i < lanes-1 {
				turn(r, -halfAngle, -1)
			}
		}
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("arcboard %.2fx%.2f %.2f %d %.2f %.2f", b.W, b.H, b.Corner, lanes, spacing, halfAngle)
	return p
}

var debugLane = true

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
