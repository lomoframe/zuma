package game

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// NewPetalSpiralOnBoard winds inwards but pinches the radius `lobes` times, so
// the route reads as a flower rather than evenly nested rings. The pinch only
// ever moves inwards, so the start still sits on the felt edge.
func NewPetalSpiralOnBoard(b Board, inset, turns, inner, lobes, amp float32) *Path {
	const samples = 2600
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	pts := make([]mgl32.Vec2, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := t * turns * 2 * math.Pi
		r := 1 + (inner-1)*t
		dip := (1 - float32(math.Cos(float64(lobes*2*math.Pi*t)))) / 2
		r *= 1 - amp*dip
		pts = append(pts, mgl32.Vec2{
			r * rx * float32(math.Cos(float64(a))),
			r * rz * float32(math.Sin(float64(a))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("petal %.2fx%.2f %.2f %.2f %.2f %.2f %.2f", b.W, b.H, inset, turns, inner, lobes, amp)
	return p
}

// NewEccentricSpiralOnBoard winds inwards around a centre that drifts, so the
// coils crowd on one side and open up on the other instead of nesting evenly.
func NewEccentricSpiralOnBoard(b Board, inset, turns, inner, drift, rot float32) *Path {
	const samples = 2600
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	c, sn := float32(math.Cos(float64(rot))), float32(math.Sin(float64(rot)))
	pts := make([]mgl32.Vec2, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := t * turns * 2 * math.Pi
		r := 1 - (1-inner)*t
		off := drift * (1 - t)
		pts = append(pts, mgl32.Vec2{
			off*c + r*rx*float32(math.Cos(float64(a))),
			off*sn + r*rz*float32(math.Sin(float64(a))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("eccentric %.2fx%.2f %.2f %.2f %.2f %.2f %.2f", b.W, b.H, inset, turns, inner, drift, rot)
	return p
}

// NewPitchSpiralOnBoard uses a non-linear radius profile: tight coils near the
// rim and a wide sweep in the middle.
func NewPitchSpiralOnBoard(b Board, inset, turns, inner, bias float32) *Path {
	const samples = 2600
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	pts := make([]mgl32.Vec2, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := t * turns * 2 * math.Pi
		r := 1 - (1-inner)*float32(math.Pow(float64(t), float64(bias)))
		pts = append(pts, mgl32.Vec2{
			r * rx * float32(math.Cos(float64(a))),
			r * rz * float32(math.Sin(float64(a))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("pitch %.2fx%.2f %.2f %.2f %.2f %.2f", b.W, b.H, inset, turns, inner, bias)
	return p
}

// NewWavyLanePath builds switchback lanes that bend instead of running straight:
// every lane is a cosine wave with zero slope at both ends, so the U-turns stay
// clean while the middle of the board sways.
func NewWavyLanePath(b Board, inset float32, lanes int, spacing, bulge float32, humps int) *Path {
	if lanes < 3 {
		lanes = 3
	}
	if humps < 2 {
		humps = 2
	}
	if humps%2 == 1 {
		humps++ // even humps keep the lane ends level with each other
	}
	h := float32(lanes-1) * spacing
	maxHalf := b.HalfH() - inset - bulge - spacing/2
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
		w := b.WidthAt(z) - inset - bulge - spacing/2
		if w < 0.12 {
			w = 0.12
		}
		return w
	}

	var pts []mgl32.Vec2
	edge := func(z float32) float32 { return z + bulge } // lane ends sit at the wave crest
	pts = append(pts, mgl32.Vec2{-halfAt(z0), edge(z0)})
	for i := 0; i < lanes; i++ {
		z := z0 + float32(i)*spacing
		w := halfAt(z)
		dir := float32(1)
		if i%2 == 1 {
			dir = -1
		}
		const steps = 120
		for k := 1; k <= steps; k++ {
			u := float32(k) / steps
			x := -w + 2*w*u
			wave := bulge * (float32(math.Cos(float64(float32(humps)*math.Pi*u))) - 1)
			pts = append(pts, mgl32.Vec2{dir * x, z + bulge + wave})
		}
		if i == lanes-1 {
			break
		}
		nz := z0 + float32(i+1)*spacing
		nw := halfAt(nz)
		// Both lane ends sit at the wave crest with zero slope, so the join is
		// the same hairpin turn the straight lanes use.
		dw := abs32(nw - w)
		if dw < 0.08 {
			cz := edge(z) + spacing/2
			const turnSteps = 30
			for k := 1; k <= turnSteps; k++ {
				a := -float32(math.Pi)/2 + float32(math.Pi)*float32(k)/turnSteps
				pts = append(pts, mgl32.Vec2{
					dir * (w + spacing/2*float32(math.Cos(float64(a)))),
					cz + spacing/2*float32(math.Sin(float64(a))),
				})
			}
			continue
		}
		k := 0.8*spacing + 0.6*dw
		from := mgl32.Vec2{dir * w, edge(z)}
		to := mgl32.Vec2{dir * nw, edge(nz)}
		p1 := mgl32.Vec2{from.X() + dir*k, from.Y()}
		p2 := mgl32.Vec2{to.X() + dir*k, to.Y()}
		const linkSteps = 26
		for j := 1; j <= linkSteps; j++ {
			t := float32(j) / linkSteps
			u := 1 - t
			pts = append(pts, from.Mul(u*u*u).
				Add(p1.Mul(3*u*u*t)).
				Add(p2.Mul(3*u*t*t)).
				Add(to.Mul(t*t*t)))
		}
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("wave %.2fx%.2f %.2f %d %.2f %.2f %d", b.W, b.H, inset, lanes, spacing, bulge, humps)
	return p
}

// NewSnailSpiralOnBoard packs the coils towards the centre: the radius falls off
// as a power of the remaining turn, so the outside is open and the middle is
// dense, like a snail shell.
func NewSnailSpiralOnBoard(b Board, inset, turns, inner, power float32) *Path {
	const samples = 3000
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	pts := make([]mgl32.Vec2, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := t * turns * 2 * math.Pi
		r := inner + (1-inner)*float32(math.Pow(float64(1-t), float64(power)))
		pts = append(pts, mgl32.Vec2{
			r * rx * float32(math.Cos(float64(a))),
			r * rz * float32(math.Sin(float64(a))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("snail %.2fx%.2f %.2f %.2f %.2f %.2f", b.W, b.H, inset, turns, inner, power)
	return p
}

// NewMoatPathOnBoard sweeps most of a lap along the rim before diving in. The
// radius profile starts flat (radius derivative 0) and blends into a constant
// pitch, so the hand-over from the lap to the spiral has no kink.
func NewMoatPathOnBoard(b Board, inset, turns, inner, lap float32) *Path {
	const samples = 3000
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	if lap < 0.1 {
		lap = 0.1
	}
	if lap > 0.6 {
		lap = 0.6
	}
	slope := 1 / (1 - lap/2)
	fall := func(t float32) float32 {
		if t < lap {
			return slope * t * t / (2 * lap)
		}
		return slope*lap/2 + slope*(t-lap)
	}
	pts := make([]mgl32.Vec2, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := t * turns * 2 * math.Pi
		r := 1 - (1-inner)*fall(t)
		pts = append(pts, mgl32.Vec2{
			r * rx * float32(math.Cos(float64(a))),
			r * rz * float32(math.Sin(float64(a))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("moat %.2fx%.2f %.2f %.2f %.2f %.2f", b.W, b.H, inset, turns, inner, lap)
	return p
}

// NewMazePath joins parallel lanes with square U-turns (two quarter arcs and a
// short straight), which reads as a maze rather than rounded switchbacks.
func NewMazePath(b Board, inset float32, lanes int, spacing, corner float32) *Path {
	if lanes < 3 {
		lanes = 3
	}
	if corner < 0.05 {
		corner = 0.05
	}
	if spacing < 2*corner+0.04 {
		corner = (spacing - 0.04) / 2
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
	// One width for every lane keeps the square turns exact.
	w := b.HalfW()
	for i := 0; i < lanes; i++ {
		if lw := b.WidthAt(z0 + float32(i)*spacing); lw < w {
			w = lw
		}
	}
	w -= inset + corner
	if w < 0.15 {
		w = 0.15
	}

	var pts []mgl32.Vec2
	// A corner is a cubic whose handles lie along both straights, so it has zero
	// curvature at the joints: no kink, no sudden tight radius.
	turn := func(a, b, tA, tB mgl32.Vec2, k float32) {
		p1 := a.Add(tA.Mul(k))
		p2 := b.Sub(tB.Mul(k))
		const steps = 14
		for i := 1; i <= steps; i++ {
			t := float32(i) / steps
			u := 1 - t
			pts = append(pts, a.Mul(u*u*u).
				Add(p1.Mul(3*u*u*t)).
				Add(p2.Mul(3*u*t*t)).
				Add(b.Mul(t*t*t)))
		}
	}
	pts = append(pts, mgl32.Vec2{-w, z0})
	for i := 0; i < lanes; i++ {
		z := z0 + float32(i)*spacing
		dir := float32(1)
		if i%2 == 1 {
			dir = -1
		}
		pts = append(pts, mgl32.Vec2{dir * w, z})
		if i == lanes-1 {
			break
		}
		k := corner * 0.55
		out := mgl32.Vec2{dir, 0}
		up := mgl32.Vec2{0, 1}
		// Out of the lane, up, then back into the next lane.
		c1 := mgl32.Vec2{dir * (w + corner), z + corner}
		turn(mgl32.Vec2{dir * w, z}, c1, out, up, k)
		c2 := mgl32.Vec2{dir * (w + corner), z + spacing - corner}
		pts = append(pts, c2)
		turn(c2, mgl32.Vec2{dir * w, z + spacing}, up, out.Mul(-1), k)
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("maze %.2fx%.2f %.2f %d %.2f %.2f %.2f", b.W, b.H, inset, lanes, spacing, corner, w)
	return p
}

// RingShape is the silhouette of one layer: a circle by default, optionally
// lobed and/or squarish. Layers do not have to nest evenly, which is the point.
type RingShape struct {
	Lobes  int     // 0 for none
	Amp    float32 // lobe depth as a fraction of the radius
	Square float32 // 0 circle .. 1 rounded square
	Rot    float32 // rotation of this layer's silhouette, radians
}

func (s RingShape) factor(a float32) float32 {
	a -= s.Rot
	f := float32(1)
	if s.Lobes > 0 && s.Amp > 0 {
		f *= 1 - s.Amp*(1-float32(math.Cos(float64(float32(s.Lobes)*a))))/2
	}
	if s.Square > 0 {
		p := float64(2 + 4*s.Square)
		c := math.Abs(math.Cos(float64(a)))
		sn := math.Abs(math.Sin(float64(a)))
		q := math.Pow(c, p) + math.Pow(sn, p)
		corner := math.Pow(2*math.Pow(math.Sqrt2/2, p), -1/p)
		f *= float32(math.Pow(q, -1/p) / corner)
	}
	return f
}

// NewLayeredRingsOnBoard builds a route out of independent rings. Each layer has
// its own silhouette and its own gap to the next one; the hand-over is a short
// arc where the radius and the silhouette both morph, so there is no seam and no
// step — the track just changes character as it comes inwards.
func NewLayeredRingsOnBoard(b Board, inset float32, shapes []RingShape, steps []float32) *Path {
	if len(shapes) < 2 {
		return NewSpiralOnBoard(b, inset, 2.8, 0.25)
	}
	rx, rz := b.HalfW()-inset, b.HalfH()-inset

	const inner = 0.14
	const minGap = 0.24
	weights := make([]float32, len(shapes)-1)
	total := float32(0)
	for i := range weights {
		w := float32(1)
		if i < len(steps) {
			w = steps[i]
		}
		weights[i] = w
		total += w
	}
	radii := make([]float32, len(shapes))
	radii[0] = 1
	acc := float32(0)
	for i := 1; i < len(shapes); i++ {
		acc += weights[i-1]
		radii[i] = 1 - (1-inner)*acc/total
	}
	// A layer whose silhouette dips inwards needs extra room below it.
	for i := 1; i < len(shapes); i++ {
		need := radii[i-1] - (minGap + shapes[i-1].Amp*radii[i-1])
		if radii[i] > need {
			radii[i] = need
		}
		if radii[i] < 0.10 {
			radii[i] = 0.10
		}
	}

	const trans = 1.15 // radians spent morphing from one layer to the next
	smooth := func(t float32) float32 { return t * t * (3 - 2*t) }

	pt := func(r, a float32) mgl32.Vec2 {
		return mgl32.Vec2{r * rx * float32(math.Cos(float64(a))), r * rz * float32(math.Sin(float64(a)))}
	}

	var pts []mgl32.Vec2
	th := float32(0)
	total2 := 2 * float32(math.Pi)
	for k := range shapes {
		from := th
		gapAng := float32(trans)
		if k == len(shapes)-1 {
			// The innermost ring must leave a real gap, otherwise the chain's
			// head and tail end up on top of each other.
			gapAng = 0.30 / (radii[k] * min32(rx, rz))
			if gapAng < float32(trans) {
				gapAng = trans
			}
			if gapAng > total2 {
				gapAng = total2
			}
		}
		ringEnd := from + total2 - gapAng
		step := 0.02 / (radii[k] * min32(rx, rz))
		if step > 0.06 {
			step = 0.06
		}
		for a := from; a <= ringEnd; a += step {
			pts = append(pts, pt(radii[k]*shapes[k].factor(a), a))
		}
		if k == len(shapes)-1 {
			pts = append(pts, pt(radii[k]*shapes[k].factor(ringEnd), ringEnd))
			break
		}
		// Morph radius and silhouette into the next layer, tangent preserved.
		const n = 26
		for i := 0; i <= n; i++ {
			t := float32(i) / n
			a := ringEnd + trans*t
			s := smooth(t)
			r := radii[k] + (radii[k+1]-radii[k])*s
			f := shapes[k].factor(a) + (shapes[k+1].factor(a)-shapes[k].factor(a))*s
			pts = append(pts, pt(r*f, a))
		}
		th = from + total2
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("layers %.2fx%.2f %.2f %d", b.W, b.H, inset, len(shapes))
	return p
}

// rectCorner draws a zero-curvature cubic corner from a to b, leaving along tA
// and arriving along tB.
func appendCorner(pts []mgl32.Vec2, a, b, tA, tB mgl32.Vec2, k float32) []mgl32.Vec2 {
	p1 := a.Add(tA.Mul(k))
	p2 := b.Sub(tB.Mul(k))
	const steps = 14
	for i := 1; i <= steps; i++ {
		t := float32(i) / steps
		u := 1 - t
		pts = append(pts, a.Mul(u*u*u).
			Add(p1.Mul(3*u*u*t)).
			Add(p2.Mul(3*u*t*t)).
			Add(b.Mul(t*t*t)))
	}
	return pts
}

// NewSquareSpiralOnBoard winds a rectangular spiral: straight legs with rounded
// corners, each lap one gap further in.
func NewSquareSpiralOnBoard(b Board, inset, gap, corner float32) *Path {
	a := b.HalfW() - inset - corner
	h := b.HalfH() - inset - corner
	for a > 0.25 && h > 0.25 {
		ok := true
		for _, z := range []float32{-h, h, -h + corner, h - corner} {
			if w := b.WidthAt(z); a+corner > w-inset+0.001 {
				ok = false
			}
		}
		if ok {
			break
		}
		a -= 0.02
		h -= 0.02
	}
	xmin, xmax := -a, a
	zmin, zmax := -h, h
	pts := []mgl32.Vec2{{xmin, zmin}}
	x, z := xmin, zmin
	turn := func(dirEnd, nextDir mgl32.Vec2) {
		k := corner * 0.55 * 1.6
		nx := mgl32.Vec2{x + dirEnd.X()*corner, z + dirEnd.Y()*corner}
		pts = appendCorner(pts, mgl32.Vec2{x, z}, nx.Add(nextDir.Mul(corner)), dirEnd, nextDir, k)
		x, z = nx.X()+nextDir.X()*corner, nx.Y()+nextDir.Y()*corner
	}
	_ = turn
	_ = x
	_ = z
	// Simple approach: four legs per lap, each one shrinking the limit behind us.
	cur := mgl32.Vec2{xmin, zmin}
	dirs := []mgl32.Vec2{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}
	leg := 0
	for lap := 0; lap < 8; lap++ {
		for k := 0; k < 4; k++ {
			d := dirs[leg%4]
			var target mgl32.Vec2
			switch leg % 4 {
			case 0:
				target = mgl32.Vec2{xmax, cur.Y()}
			case 1:
				target = mgl32.Vec2{cur.X(), zmax}
			case 2:
				target = mgl32.Vec2{xmin, cur.Y()}
			case 3:
				target = mgl32.Vec2{cur.X(), zmin}
			}
			// Stop the straight part short of the corner.
			stop := target.Sub(d.Mul(corner))
			pts = append(pts, stop)
			nextDir := dirs[(leg+1)%4]
			cornerEnd := stop.Add(d.Mul(corner)).Add(nextDir.Mul(corner))
			pts = appendCorner(pts, stop, cornerEnd, d, nextDir, corner*0.55)
			cur = cornerEnd
			switch leg % 4 {
			case 0:
				xmin += gap
			case 1:
				zmin += gap
			case 2:
				xmax -= gap
			case 3:
				zmax -= gap
			}
			leg++
			if xmax-xmin < 2*gap || zmax-zmin < 2*gap {
				pts = append(pts, cur)
				p := NewPath(pts)
				p.Key = fmt.Sprintf("square %.2fx%.2f %.2f %.2f %.2f", b.W, b.H, inset, gap, corner)
				return p
			}
		}
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("square %.2fx%.2f %.2f %.2f %.2f", b.W, b.H, inset, gap, corner)
	return p
}

// NewTwinArmSpiralOnBoard winds one arm in to the middle and a second arm back
// out, keeping the two arms exactly half a turn apart so they never touch.
func NewTwinArmSpiralOnBoard(b Board, inset, turns, inner float32) *Path {
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	pt := func(r, a float32) mgl32.Vec2 {
		return mgl32.Vec2{r * rx * float32(math.Cos(float64(a))), r * rz * float32(math.Sin(float64(a)))}
	}
	// angle at which a given radius sits on the inbound arm
	angleAt := func(r float32) float32 {
		t := (1 - r) / (1 - inner)
		return t * turns * 2 * math.Pi
	}
	var pts []mgl32.Vec2
	const steps = 1200
	for i := 0; i <= steps; i++ {
		t := float32(i) / steps
		r := 1 - (1-inner)*t
		pts = append(pts, pt(r, t*turns*2*math.Pi))
	}
	// Half a turn around the hub onto the outbound arm.
	hubA := angleAt(inner)
	const hubSteps = 64
	for i := 1; i <= hubSteps; i++ {
		t := float32(i) / hubSteps
		pts = append(pts, pt(inner, hubA+t*float32(math.Pi)))
	}
	// Outbound: same radius/angle relation shifted by half a turn, so the arms
	// stay opposite each other the whole way out.
	for i := 1; i <= steps; i++ {
		u := float32(i) / steps
		r := inner + (1-inner)*u
		pts = append(pts, pt(r, angleAt(r)+float32(math.Pi)))
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("twinarm %.2fx%.2f %.2f %.2f %.2f", b.W, b.H, inset, turns, inner)
	return p
}

// NewLoopChainOnBoard strings independent loops across the board: each loop is
// walked almost all the way round, then a short tangent link steps to the next.
func NewLoopChainOnBoard(b Board, inset float32, loops int, loopR float32) *Path {
	if loops < 2 {
		loops = 2
	}
	// Loops sit side by side, so their diameter has to fit between the centres.
	if maxR := 0.8 * (b.HalfW() - inset) / float32(loops); loopR > maxR {
		loopR = maxR
	}
	if maxR := b.HalfH() - inset; loopR > maxR {
		loopR = maxR
	}
	halfW := b.HalfW() - inset - loopR
	var pts []mgl32.Vec2
	prevEnd := mgl32.Vec2{}
	prevTan := mgl32.Vec2{}
	for i := 0; i < loops; i++ {
		var cx float32
		if loops == 1 {
			cx = 0
		} else {
			cx = -halfW + 2*halfW*float32(i)/float32(loops-1)
		}
		cy := float32(0)
		start := float32(i) * 0.9
		gap := float32(0.9)
		const steps = 90
		if i > 0 {
			// Link from the previous loop's end into this loop's start.
			a := start
			entry := mgl32.Vec2{cx + loopR*float32(math.Cos(float64(a))), cy + loopR*float32(math.Sin(float64(a)))}
			tan := mgl32.Vec2{-float32(math.Sin(float64(a))), float32(math.Cos(float64(a)))}
			dist := entry.Sub(prevEnd).Len()
			k := dist * 0.4
			pts = appendCorner(pts, prevEnd, entry, prevTan, tan, k)
		}
		for s := 0; s < steps; s++ {
			a := start + (2*float32(math.Pi)-gap)*float32(s)/steps
			pts = append(pts, mgl32.Vec2{cx + loopR*float32(math.Cos(float64(a))), cy + loopR*float32(math.Sin(float64(a)))})
		}
		endA := start + 2*float32(math.Pi) - gap
		prevEnd = mgl32.Vec2{cx + loopR*float32(math.Cos(float64(endA))), cy + loopR*float32(math.Sin(float64(endA)))}
		prevTan = mgl32.Vec2{-float32(math.Sin(float64(endA))), float32(math.Cos(float64(endA)))}
		pts = append(pts, prevEnd)
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("loopchain %.2fx%.2f %.2f %d %.2f", b.W, b.H, inset, loops, loopR)
	return p
}

// profileSpiral winds a spiral whose radius follows a normalised silhouette, so
// the same route reads as a doodle (heart, star, ...) while closing inwards.
func profileSpiral(b Board, inset, turns, inner float32, prof func(a float32) float32, key string) *Path {
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	const samples = 2600
	pts := make([]mgl32.Vec2, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float32(i) / samples
		a := t * turns * 2 * math.Pi
		r := (1 - (1-inner)*t) * prof(a)
		pts = append(pts, mgl32.Vec2{
			r * rx * float32(math.Cos(float64(a))),
			r * rz * float32(math.Sin(float64(a))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("%s %.2fx%.2f %.2f %.2f %.2f", key, b.W, b.H, inset, turns, inner)
	return p
}

var heartTable = func() [720]float32 {
	var table [720]float32
	maxR := float32(0.0001)
	for i := 0; i < 4096; i++ {
		t := float64(i) / 4096 * 2 * math.Pi
		x := 16 * math.Pow(math.Sin(t), 3)
		y := 13*math.Cos(t) - 5*math.Cos(2*t) - 2*math.Cos(3*t) - math.Cos(4*t)
		r := math.Hypot(x, y)
		a := math.Atan2(y, x)
		if a < 0 {
			a += 2 * math.Pi
		}
		idx := int(a / (2 * math.Pi) * 720)
		if idx >= 720 {
			idx = 719
		}
		if float32(r) > table[idx] {
			table[idx] = float32(r)
		}
	}
	for _, v := range table {
		if v > maxR {
			maxR = v
		}
	}
	for i := range table {
		table[i] /= maxR
	}
	return table
}()

func heartProfile(a float32) float32 {
	idx := int((a / (2 * math.Pi)) * 720)
	idx = ((idx % 720) + 720) % 720
	// Keep part of the nominal radius so the heart's cleft cannot fold the laps
	// on top of each other.
	return 0.62 + 0.38*heartTable[idx]
}

// starProfile is a five-pointed star with rounded tips.
func starProfile(a float32) float32 {
	return 1 - 0.42*(1-float32(math.Cos(float64(5*a))))/2
}

// NewHeartSpiralOnBoard draws the route as a heart that keeps winding inwards.
func NewHeartSpiralOnBoard(b Board, inset, turns, inner float32) *Path {
	return profileSpiral(b, inset, turns, inner, heartProfile, "heart")
}

// NewStarSpiralOnBoard draws the route as a rounded five-pointed star.
func NewStarSpiralOnBoard(b Board, inset, turns, inner float32) *Path {
	return profileSpiral(b, inset, turns, inner, starProfile, "star")
}

// chaikin rounds a closed polyline by corner cutting, so a hand-drawn silhouette
// becomes a smooth outline.
func chaikin(p []mgl32.Vec2, iters int) []mgl32.Vec2 {
	for ; iters > 0; iters-- {
		out := make([]mgl32.Vec2, 0, 2*len(p))
		for i := range p {
			a, b := p[i], p[(i+1)%len(p)]
			out = append(out,
				a.Mul(0.75).Add(b.Mul(0.25)),
				a.Mul(0.25).Add(b.Mul(0.75)))
		}
		p = out
	}
	return p
}

// NewDoodleSpiralOnBoard traces a hand-drawn silhouette, winding it inwards.
// The outline is given in unit coordinates; it is smoothed, normalised to radius
// one and then scaled down turn after turn.
func NewDoodleSpiralOnBoard(b Board, inset, turns, inner float32, key string, outline []mgl32.Vec2) *Path {
	return NewDoodleSpiralRounded(b, inset, turns, inner, key, outline, 2)
}

// NewDoodleSpiralRounded is the same with an explicit number of rounding passes;
// polygons need one pass to keep their corners recognisable.
func NewDoodleSpiralRounded(b Board, inset, turns, inner float32, key string, outline []mgl32.Vec2, rounds int) *Path {
	return doodleSpiral(b, inset, turns, inner, key, outline, rounds, 0, 0, 0)
}

// NewDoodleSpiralDrift keeps the most off-centre version that still passes the
// bead-spacing guard: larger twists and drifts are tried first and dialled back
// only when two laps would come closer than one bead.
func NewDoodleSpiralDrift(b Board, inset, turns, inner float32, key string, outline []mgl32.Vec2, rounds int) *Path {
	dirs := [][2]float32{{1, 0.2}, {-1, 0.4}, {0.3, 1}, {0.4, -1}}
	for _, tw := range []float32{0.9, 0.55, 0.3, 0} {
		for _, d := range []float32{0.40, 0.30, 0.22, 0.14, 0.08, 0} {
			for _, dir := range dirs {
				p := doodleSpiral(b, inset, turns, inner, key, outline, rounds, tw, d*dir[0], d*dir[1])
				if ValidateRoute(b, p) == nil {
					return p
				}
			}
		}
	}
	// Nothing decorative fits: fall back to fewer laps, larger inner radius.
	for _, tr := range []float32{2.2, 1.8, 1.5, 1.2} {
		for _, in := range []float32{0.40, 0.50, 0.60} {
			p := doodleSpiral(b, inset, tr, in, key, outline, rounds, 0, 0, 0)
			if ValidateRoute(b, p) == nil {
				return p
			}
		}
	}
	return doodleSpiral(b, inset, 1.2, 0.6, key, outline, rounds, 0, 0, 0)
}

// doodleSpiral redraws the outline smaller while rotating and shifting it.
func doodleSpiral(b Board, inset, turns, inner float32, key string, outline []mgl32.Vec2, rounds int, twist, driftX, driftZ float32) *Path {
	shape := chaikin(outline, rounds)
	maxR := float32(0.001)
	for _, p := range shape {
		if l := p.Len(); l > maxR {
			maxR = l
		}
	}
	minR := float32(9)
	for i := range shape {
		shape[i] = shape[i].Mul(1 / maxR)
		if l := shape[i].Len(); l < minR {
			minR = l
		}
	}
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	n := len(shape)
	const total = 4200
	pts := make([]mgl32.Vec2, 0, total+1)
	for k := 0; k <= total; k++ {
		u := float32(k) / total
		s := 1 - (1-inner)*u
		f := float64(u * turns * float32(n))
		i0 := int(f) % n
		i1 := (i0 + 1) % n
		w := float32(f - math.Trunc(f))
		p := shape[i0].Mul(1 - w).Add(shape[i1].Mul(w))
		ang := twist * u
		ca, sa := float32(math.Cos(float64(ang))), float32(math.Sin(float64(ang)))
		px := (p.X()*ca - p.Y()*sa) * s * rx
		pz := (p.X()*sa + p.Y()*ca) * s * rz
		pts = append(pts, mgl32.Vec2{px + driftX*u, pz + driftZ*u})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("%s %.2fx%.2f %.2f %.2f %.2f", key, b.W, b.H, inset, turns, inner)
	return p
}

func appleOutline() []mgl32.Vec2 {
	const n = 72
	out := make([]mgl32.Vec2, 0, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / n
		// round body with a dent at the top and a small dimple at the bottom
		r := 1.0
		top := math.Exp(-math.Pow(a-math.Pi/2, 2) / (2 * 0.20 * 0.20))
		bot := math.Exp(-math.Pow(a+math.Pi/2, 2) / (2 * 0.30 * 0.30))
		r -= 0.18*top + 0.10*bot
		// apples are a little wider than tall
		out = append(out, mgl32.Vec2{float32(1.12 * r * math.Cos(a)), float32(r * math.Sin(a))})
	}
	return out
}

func pentagonOutline() []mgl32.Vec2 {
	out := make([]mgl32.Vec2, 0, 5)
	for i := 0; i < 5; i++ {
		a := math.Pi/2 + 2*math.Pi*float64(i)/5
		out = append(out, mgl32.Vec2{float32(math.Cos(a)), float32(math.Sin(a))})
	}
	return out
}

func gamepadOutline() []mgl32.Vec2 {
	return []mgl32.Vec2{
		{-0.95, 0.05}, {-0.86, -0.22}, {-0.52, -0.30}, {-0.30, -0.16},
		{0.30, -0.16}, {0.52, -0.30}, {0.86, -0.22}, {0.95, 0.05},
		{0.70, 0.36}, {0.24, 0.32}, {0, 0.22}, {-0.24, 0.32}, {-0.70, 0.36},
	}
}

func headsetOutline() []mgl32.Vec2 {
	return []mgl32.Vec2{
		{-0.92, 0.06}, {-0.88, -0.26}, {-0.50, -0.42}, {-0.22, -0.30},
		{0.22, -0.30}, {0.50, -0.42}, {0.88, -0.26}, {0.92, 0.06},
		{0.60, 0.46}, {-0.60, 0.46},
	}
}

func heartOutline() []mgl32.Vec2 {
	const n = 96
	out := make([]mgl32.Vec2, 0, n)
	for i := 0; i < n; i++ {
		t := 2 * math.Pi * float64(i) / n
		x := 16 * math.Pow(math.Sin(t), 3)
		y := 13*math.Cos(t) - 5*math.Cos(2*t) - 2*math.Cos(3*t) - math.Cos(4*t)
		out = append(out, mgl32.Vec2{float32(x / 16), float32(y / 16)})
	}
	return out
}

func starOutline() []mgl32.Vec2 {
	const n = 10
	out := make([]mgl32.Vec2, 0, n)
	for i := 0; i < n; i++ {
		a := math.Pi/2 + math.Pi*float64(i)/5
		r := 1.0
		if i%2 == 1 {
			r = 0.42
		}
		out = append(out, mgl32.Vec2{float32(r * math.Cos(a)), float32(r * math.Sin(a))})
	}
	return out
}

// NewDoodlePathOnBoard draws a doodle once as a single stroke, then lets the
// line dissolve into a spiral inside it. The outline is only traced a lap, so
// concave silhouettes (grips, notches) cannot fold onto themselves.
func NewDoodlePathOnBoard(b Board, inset float32, key string, outline []mgl32.Vec2, rounds int, turns, inner float32) *Path {
	shape := chaikin(outline, rounds)
	maxR := float32(0.001)
	for _, p := range shape {
		if l := p.Len(); l > maxR {
			maxR = l
		}
	}
	minR := float32(9)
	for i := range shape {
		shape[i] = shape[i].Mul(1 / maxR)
		if l := shape[i].Len(); l < minR {
			minR = l
		}
	}
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	n := len(shape)
	at := func(t float32) (float32, float32) {
		f := float64(t * float32(n))
		i0 := int(f) % n
		i1 := (i0 + 1) % n
		w := float32(f - math.Trunc(f))
		p := shape[i0].Mul(1 - w).Add(shape[i1].Mul(w))
		return p.X(), p.Y()
	}
	const lap = 2 * float32(math.Pi)
	const trans = 0.75
	// The inner spiral has to fit inside the silhouette's narrowest radius,
	// otherwise it would cut across a cleft or a star point.
	s0 := minR - 0.12
	if s0 > 0.55 {
		s0 = 0.55
	}
	if s0 < 0.20 {
		s0 = 0.20
	}
	// Coils must stay at least one bead apart: derive the turn count and the
	// final radius from the space actually available inside the silhouette.
	minPitch := 0.22 / rz
	maxTurns := (s0 - 0.12) / minPitch
	if turns > maxTurns {
		turns = maxTurns
	}
	if turns < 0.9 {
		turns = 0.9
	}
	inner = s0 - turns*minPitch
	if inner < 0.06 {
		inner = 0.06
	}
	smooth := func(u float32) float32 { return u * u * (3 - 2*u) }

	var pts []mgl32.Vec2
	const n1 = 1300
	for i := 0; i <= n1; i++ {
		t := float32(i) / n1 * (lap - trans) / lap
		x, z := at(t)
		pts = append(pts, mgl32.Vec2{x * rx, z * rz})
	}
	const m = 90
	for i := 1; i <= m; i++ {
		u := float32(i) / m
		sm := smooth(u)
		ang := (lap - trans) + trans*u
		x, z := at(ang / lap)
		ca, sa := float32(math.Cos(float64(ang))), float32(math.Sin(float64(ang)))
		px := x*(1-sm) + ca*sm
		pz := z*(1-sm) + sa*sm
		s := 1 + (s0-1)*sm
		pts = append(pts, mgl32.Vec2{px * s * rx, pz * s * rz})
	}
	const n3 = 1600
	for i := 1; i <= n3; i++ {
		u := float32(i) / n3
		ang := lap + u*turns*lap
		s := s0 - (s0-inner)*u
		pts = append(pts, mgl32.Vec2{
			s * rx * float32(math.Cos(float64(ang))),
			s * rz * float32(math.Sin(float64(ang))),
		})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("doodle %s %.2fx%.2f %.2f %.2f %.2f", key, b.W, b.H, inset, turns, inner)
	return p
}

func eyeOutline() []mgl32.Vec2 {
	const n = 64
	out := make([]mgl32.Vec2, 0, 2*n)
	for i := 0; i < n; i++ { // upper lid, left tip to right tip
		t := math.Pi * float64(i) / n
		out = append(out, mgl32.Vec2{float32(-math.Cos(t)), float32(0.52 * math.Sin(t))})
	}
	for i := 0; i < n; i++ { // lower lid back
		t := math.Pi * float64(i) / n
		out = append(out, mgl32.Vec2{float32(math.Cos(t)), float32(-0.52 * math.Sin(t))})
	}
	return out
}

func cupOutline() []mgl32.Vec2 {
	// A plain tumbler: single closed silhouette, so it stays star-shaped and can
	// be wound inwards without the walls touching.
	return []mgl32.Vec2{
		{-0.60, 0.85}, {-0.42, -0.78}, {0.42, -0.78}, {0.60, 0.85},
	}
}

func bananaOutline() []mgl32.Vec2 {
	const n = 40
	out := make([]mgl32.Vec2, 0, 2*n)
	for i := 0; i <= n; i++ { // outer belly
		t := math.Pi * float64(i) / n
		out = append(out, mgl32.Vec2{float32(-0.95 * math.Cos(t)), float32(0.75*math.Sin(t) - 0.15)})
	}
	for i := 0; i <= n; i++ { // inner back
		t := math.Pi * float64(n-i) / n
		out = append(out, mgl32.Vec2{float32(-0.80 * math.Cos(t)), float32(0.30*math.Sin(t) - 0.05)})
	}
	return out
}

// Doodle returns a named one-stroke silhouette and how many rounding passes it
// needs. ok is false for unknown names.
func Doodle(name string) ([]mgl32.Vec2, int, bool) {
	switch name {
	case "heart":
		return heartOutline(), 2, true
	case "star":
		return starOutline(), 1, true
	case "apple":
		return appleOutline(), 2, true
	case "penta":
		return pentagonOutline(), 1, true
	case "eye":
		return eyeOutline(), 2, true
	case "cup":
		return cupOutline(), 1, true
	case "banana":
		return bananaOutline(), 2, true
	case "pad":
		return gamepadOutline(), 2, true
	}
	return nil, 0, false
}

// NewDoodleDoubleLine traces the silhouette twice at a steady spacing: an outer
// pass and one slightly smaller copy, ending at the hole. No twist, no drift —
// the shape stays readable instead of turning into scribble.
func NewDoodleDoubleLine(b Board, inset, turns, inner float32, key string, outline []mgl32.Vec2, rounds int) *Path {
	return doodleSpiral(b, inset, turns, inner, key, outline, rounds, 0, 0, 0)
}

// NewDoodleOutline traces a silhouette exactly once and stops at the hole, which
// is what a one-stroke drawing is: the outline, nothing nested inside it. A
// bead-sized gap is left at the seam so the chain cannot touch its own start.
func NewDoodleOutline(b Board, inset float32, key string, outline []mgl32.Vec2, rounds int) *Path {
	shape := chaikin(outline, rounds)
	maxR := float32(0.001)
	for _, p := range shape {
		if l := p.Len(); l > maxR {
			maxR = l
		}
	}
	for i := range shape {
		shape[i] = shape[i].Mul(1 / maxR)
	}
	// Pull the silhouette a little towards a circle: a deep cleft (heart) or a
	// star point otherwise brings two laps of the same outline too close.
	for i := range shape {
		if l := shape[i].Len(); l > 0.001 {
			dir := shape[i].Mul(1 / l)
			shape[i] = shape[i].Mul(0.50).Add(dir.Mul(0.50))
		}
	}
	rx, rz := b.HalfW()-inset, b.HalfH()-inset
	n := len(shape)

	total := 0.0
	for i := range shape {
		a, c := shape[i], shape[(i+1)%n]
		total += math.Hypot(float64((c.X()-a.X())*rx), float64((c.Y()-a.Y())*rz))
	}
	gapFrac := 0.55 / total
	if gapFrac > 0.12 {
		gapFrac = 0.12
	}

	const steps = 2400
	pts := make([]mgl32.Vec2, 0, steps+1)
	for k := 0; k <= steps; k++ {
		u := float32(k) / steps * (1 - float32(gapFrac))
		f := float64(u * float32(n))
		i0 := int(f) % n
		i1 := (i0 + 1) % n
		w := float32(f - math.Trunc(f))
		p := shape[i0].Mul(1 - w).Add(shape[i1].Mul(w))
		pts = append(pts, mgl32.Vec2{p.X() * rx, p.Y() * rz})
	}
	p := NewPath(pts)
	p.Key = fmt.Sprintf("outline %s %.2fx%.2f %.2f", key, b.W, b.H, inset)
	return p
}
