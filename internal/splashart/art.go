// Package splashart paints the loading screen art: a jungle temple with a stone
// medallion whose spiral groove is where the renderer animates the bead chain.
// The image is generated at build-dev time by tools/splashart and embedded, so
// the game keeps its single-binary shape.
package splashart

import (
	"image"
	"math"
)

// Art size in pixels. Everything in this package and in render/splash.go is
// expressed in these art pixels, with y growing downwards like image rows.
const (
	ArtW = 1920
	ArtH = 1080
)

// The medallion and its spiral groove. render/splash.go walks the same spiral
// with SpiralAt, so the animated beads sit exactly in the painted groove.
const (
	CenterX = 1292.0
	CenterY = 556.0

	SpiralOuter = 252.0
	SpiralInner = 64.0
	Turns       = 1.8

	GrooveHalf  = 27.0 // half width of the dark channel
	BeadRadius  = 23.0
	BeadSpacing = 2*BeadRadius + 4
)

// Palette.
var (
	ink       = col{0.012, 0.020, 0.026}
	gold      = col{0.94, 0.76, 0.38}
	goldDeep  = col{0.62, 0.42, 0.14}
	jade      = col{0.055, 0.150, 0.150}
	jadeLight = col{0.10, 0.27, 0.26}
	stone     = col{0.145, 0.145, 0.130}
	stoneHi   = col{0.22, 0.215, 0.19}
	leafDark  = col{0.010, 0.030, 0.026}
	leafRim   = col{0.055, 0.145, 0.115}
)

type col struct{ r, g, b float64 }

func (c col) mul(f float64) col { return col{c.r * f, c.g * f, c.b * f} }
func (c col) add(o col) col     { return col{c.r + o.r, c.g + o.g, c.b + o.b} }
func mix(a, b col, t float64) col {
	return col{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}
func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
func smooth(e0, e1, x float64) float64 {
	t := clamp01((x - e0) / (e1 - e0))
	return t * t * (3 - 2*t)
}

// hash1 is a cheap deterministic value noise in [0,1).
func hash1(x, y float64) float64 {
	v := math.Sin(x*127.1+y*311.7) * 43758.5453
	return v - math.Floor(v)
}

// valueNoise interpolates the hash on a lattice, giving the stone its grain.
func valueNoise(x, y float64) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	fx, fy := x-xi, y-yi
	sx := fx * fx * (3 - 2*fx)
	sy := fy * fy * (3 - 2*fy)
	a := hash1(xi, yi)
	b := hash1(xi+1, yi)
	c := hash1(xi, yi+1)
	d := hash1(xi+1, yi+1)
	return a*(1-sx)*(1-sy) + b*sx*(1-sy) + c*(1-sx)*sy + d*sx*sy
}

// spiral table: cumulative arc length of the painted groove.
var (
	spX, spY, spS []float32
	SpiralLength  float32
)

func init() {
	const n = 1400
	spX = make([]float32, n+1)
	spY = make([]float32, n+1)
	spS = make([]float32, n+1)
	var px, py float64
	for i := 0; i <= n; i++ {
		u := float64(i) / n
		th := 2 * math.Pi * Turns * u
		r := SpiralOuter + (SpiralInner-SpiralOuter)*u
		x := CenterX + r*math.Cos(th)
		y := CenterY + r*math.Sin(th)
		if i > 0 {
			SpiralLength += float32(math.Hypot(x-px, y-py))
		}
		spX[i], spY[i], spS[i] = float32(x), float32(y), SpiralLength
		px, py = x, y
	}
}

// SpiralAt maps an arc length in pixels to a point on the painted groove. ok is
// false once s runs past the inner end, which is where the beads get swallowed.
func SpiralAt(s float32) (x, y float32, ok bool) {
	if s < 0 || s > SpiralLength {
		return 0, 0, false
	}
	lo, hi := 0, len(spS)-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if spS[mid] <= s {
			lo = mid
		} else {
			hi = mid
		}
	}
	span := spS[hi] - spS[lo]
	t := float32(0)
	if span > 0 {
		t = (s - spS[lo]) / span
	}
	return spX[lo] + (spX[hi]-spX[lo])*t, spY[lo] + (spY[hi]-spY[lo])*t, true
}

// spiralAt returns the groove point plus how far the sample is from the painted
// centre line, for painting the channel per pixel.
func spiralAt(px, py float64) (dist, u float64, ok bool) {
	dx, dy := px-CenterX, py-CenterY
	r := math.Hypot(dx, dy)
	if r > SpiralOuter+GrooveHalf+6 || r < SpiralInner-GrooveHalf-6 {
		return 0, 0, false
	}
	a := math.Atan2(dy, dx)
	best := 1e9
	for k := 0; k <= int(math.Ceil(Turns)); k++ {
		th := a + 2*math.Pi*float64(k)
		if th < 0 {
			th += 2 * math.Pi
		}
		uu := th / (2 * math.Pi * Turns)
		if uu < 0 || uu > 1 {
			continue
		}
		rr := SpiralOuter + (SpiralInner-SpiralOuter)*uu
		if d := math.Abs(r - rr); d < best {
			best, u = d, uu
		}
	}
	if best > 1e8 {
		return 0, 0, false
	}
	return best, u, true
}

// Render paints the loading background.
func Render() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, ArtW, ArtH))
	for y := 0; y < ArtH; y++ {
		for x := 0; x < ArtW; x++ {
			c, a := shade(float64(x)+0.5, float64(y)+0.5)
			o := img.PixOffset(x, y)
			img.Pix[o+0] = byte(255 * clamp01(c.r) * a)
			img.Pix[o+1] = byte(255 * clamp01(c.g) * a)
			img.Pix[o+2] = byte(255 * clamp01(c.b) * a)
			img.Pix[o+3] = 255
		}
	}
	return img
}

// shade composites every layer for one pixel. Layer order is back to front:
// sky, temple, medallion, foreground leaves, vignette and grain.
func shade(px, py float64) (col, float64) {
	// -- sky: vertical gradient plus the warm glow behind the medallion.
	v := py / ArtH
	c := mix(col{0.020, 0.038, 0.052}, col{0.014, 0.052, 0.058}, smooth(0, 0.55, v))
	c = mix(c, col{0.006, 0.014, 0.018}, smooth(0.55, 1.0, v))

	dx, dy := px-CenterX, py-(CenterY-140)
	d := math.Hypot(dx, dy)
	halo := math.Pow(clamp01(1-d/900), 2.4)
	c = c.add(gold.mul(0.38 * halo))
	c = c.add(col{0.05, 0.14, 0.13}.mul(math.Pow(clamp01(1-d/1500), 2.0)))

	// -- light shafts fanning down from above the medallion.
	sx, sy := CenterX, -120.0
	ax, ay := px-sx, py-sy
	ang := math.Atan2(ay, ax)
	dist := math.Hypot(ax, ay)
	for i := 0; i < 7; i++ {
		center := -math.Pi/2 + (float64(i)-3)*0.20
		width := 0.030 + 0.020*math.Sin(float64(i)*1.7+0.4)
		w := 1 - smooth(width, width*2.3, math.Abs(ang-center))
		if w <= 0 {
			continue
		}
		fade := math.Pow(clamp01(1-dist/1500), 1.6) * 0.20
		c = c.add(gold.mul(w * fade))
	}

	// -- temple floor: wide stone courses at the bottom edge.
	if py > 1004 {
		row := math.Floor((py - 1004) / 44)
		joint := math.Mod(math.Floor((px+row*90)/190), 2)
		n := valueNoise(px/30, py/30)
		c = col{0.026, 0.038, 0.038}.mul(0.78 + 0.34*n)
		if math.Mod(py-1004, 44) < 2 {
			c = c.add(col{0.05, 0.06, 0.05}.mul(0.9))
		}
		if math.Mod(px+row*95, 190) < 2 && joint >= 0 {
			c = c.mul(0.72)
		}
		if py < 1008 {
			c = c.add(goldDeep.mul(0.25))
		}
	}
	// -- medallion: outer shadow, gold rim, stone ring, jade felt, groove.
	mx, my := px-CenterX, py-CenterY
	r := math.Hypot(mx, my)
	if r < 336 {
		n := valueNoise(px/9, py/9)
		switch {
		case r > 318:
			c = mix(c, ink, smooth(336, 318, r)*0.85)
		case r > 310:
			// gold rim, brighter where the light hits the upper left.
			lit := clamp01(0.55 - 0.45*(mx+my)/340)
			c = mix(c, gold.mul(0.62+0.55*lit).mul(0.9+0.2*n), smooth(310, 314, r))
		case r > 286:
			// stone ring with a bevel highlight on its outer edge.
			b := mix(stone.mul(0.55), stoneHi, smooth(288, 306, r))
			c = b.mul(0.85 + 0.30*n)
			if r > 309 && r < 311 {
				c = c.add(goldDeep.mul(0.35))
			}
		default:
			felt := mix(jade.mul(0.62), jade, smooth(40, 283, r))
			c = felt.mul(0.94 + 0.12*n)
			// spiral groove
			if d, u, ok := spiralAt(px, py); ok {
				lip := math.Abs(d) - GrooveHalf
				switch {
				case lip < 0:
					c = mix(c, col{0.010, 0.020, 0.024}, smooth(0, 4, -lip)*0.94)
					c = c.add(jadeLight.mul(0.05 * (1 - clamp01(-lip/GrooveHalf))))
				case lip < 4:
					c = mix(c, jadeLight.mul(0.72), 0.55*(1-lip/4))
				}
				// the far end of the spiral fades into the idol
				_ = u
			}
		}
	}

	// -- the stone frog idol sits above the spiral's inner end, so the chain
	// wraps under it and disappears into the drain at its feet.
	ell := func(cx, cy, rx, ry float64) float64 {
		return math.Hypot((px-cx)/rx, (py-cy)/ry)
	}
	if r < 120 {
		c = c.add(gold.mul(0.30 * math.Pow(clamp01(1-r/120), 2.0)))
	}
	body := ell(CenterX, CenterY+12, 54, 35)
	head := ell(CenterX, CenterY-24, 37, 27)
	legL := ell(CenterX-44, CenterY+32, 21, 15)
	legR := ell(CenterX+44, CenterY+32, 21, 15)
	if body < 1 || head < 1 || legL < 1 || legR < 1 {
		n := valueNoise(px/6, py/6)
		c = mix(col{0.052, 0.130, 0.112}, col{0.086, 0.205, 0.168}, 0.45+0.55*n)
		if py < CenterY+8 {
			c = c.add(leafRim.mul(0.30))
		}
		if body > 0.95 || head > 0.95 {
			c = c.add(goldDeep.mul(0.30))
		}
	}
	// wide stone mouth between the head and the body.
	if ell(CenterX, CenterY-8, 30, 7) < 1 {
		c = mix(c, ink.mul(1.3), 0.95)
	}
	// nostrils.
	for _, nx := range []float64{-7, 7} {
		if math.Hypot(px-(CenterX+nx), py-(CenterY-40)) < 2 {
			c = mix(c, ink.mul(1.4), 1)
		}
	}
	// eyes on top of the head.
	for _, e := range []struct{ dx, dy float64 }{{-21, -56}, {21, -56}} {
		ed := math.Hypot(px-(CenterX+e.dx), py-(CenterY+e.dy))
		if ed < 12 {
			c = mix(c, col{0.070, 0.185, 0.160}, 1)
			c = c.add(goldDeep.mul(0.25 * smooth(12, 8, ed)))
		}
		if ed < 7.5 {
			c = mix(c, gold.mul(0.92), 1)
		}
		if ed < 3.2 {
			c = mix(c, ink.mul(1.2), 1)
		}
	}
	// the drain the chain falls into, right at the inner end of the groove.
	dxEnd, dyEnd := CenterX, CenterY+SpiralInner
	if de := math.Hypot(px-dxEnd, py-dyEnd); de < 40 {
		n := valueNoise(px/5, py/5)
		switch {
		case de < 27:
			c = mix(c, col{0.004, 0.007, 0.009}, 1)
			c = c.add(col{0.01, 0.02, 0.02}.mul(0.6 * n))
		case de < 36:
			c = mix(c, stone.mul(0.55).mul(0.9+0.3*n), smooth(27, 32, de))
			if de > 34 {
				c = c.add(goldDeep.mul(0.30))
			}
		}
	}
	// -- the entrance the chain pours out of, at the outer end of the groove.
	ex0, ey0 := CenterX+SpiralOuter, CenterY
	if e := math.Hypot(px-ex0, py-ey0); e < 36 {
		n := valueNoise(px/5, py/5)
		switch {
		case e < 22:
			c = mix(c, col{0.005, 0.009, 0.011}, 1)
			c = c.add(col{0.012, 0.024, 0.024}.mul(0.5 * n))
		case e < 32:
			c = mix(c, stoneHi.mul(0.45).mul(0.9+0.3*n), smooth(22, 27, e))
			if e > 30 {
				c = c.add(gold.mul(0.22))
			}
		}
	}

	// -- carved studs around the ring.
	for i := 0; i < 6; i++ {
		a := float64(i)*math.Pi/3 + math.Pi/6
		ex := CenterX + 298*math.Cos(a)
		ey := CenterY + 298*math.Sin(a)
		ed := math.Hypot(px-ex, py-ey)
		if ed < 11 {
			c = mix(c, stoneHi.mul(0.75+0.35*valueNoise(px/4, py/4)), smooth(11, 8, ed))
			if ed < 4 {
				c = c.add(goldDeep.mul(0.25))
			}
		}
	}

	// -- title plate: a quiet stone tablet behind the (runtime drawn) title.
	if plateIn(px, py) {
		n := valueNoise(px/22, py/22)
		c = mix(c, col{0.022, 0.042, 0.044}.mul(0.85+0.3*n), 0.55)
		if nearPlateEdge(px, py) {
			c = c.add(gold.mul(0.20))
		}
	}

	// -- foreground jungle leaves.
	for _, l := range leaves {
		if a := l.coverage(px, py); a > 0 {
			n := valueNoise(px/7, py/7)
			lc := leafDark.mul(0.7 + 0.6*n)
			if l.rim(px, py) {
				lc = lc.add(leafRim.mul(0.55))
			}
			c = mix(c, lc, a)
		}
	}

	// -- vignette and film grain.
	vd := math.Hypot(px-ArtW/2, py-ArtH/2)
	c = c.mul(1 - 0.62*math.Pow(clamp01(vd/1150), 2.1))
	c = c.add(col{1, 1, 1}.mul(0.012 * (hash1(px, py) - 0.5)))
	if c.r < 0 {
		c.r = 0
	}
	if c.g < 0 {
		c.g = 0
	}
	if c.b < 0 {
		c.b = 0
	}
	return c, 1
}

// plateIn reports whether a pixel falls inside the title tablet.
func plateIn(px, py float64) bool {
	const x0, y0, x1, y1 = 116.0, 170.0, 806.0, 486.0
	const rr = 24.0
	if px < x0 || px > x1 || py < y0 || py > y1 {
		return false
	}
	cx := math.Max(x0+rr, math.Min(x1-rr, px))
	cy := math.Max(y0+rr, math.Min(y1-rr, py))
	return math.Hypot(px-cx, py-cy) <= rr
}

func nearPlateEdge(px, py float64) bool {
	for _, o := range []struct{ x, y float64 }{{0, 0}, {0, 6}, {-6, 0}, {6, 0}, {0, -6}} {
		if plateIn(px+o.x, py+o.y) && !plateIn(px, py) {
			return true
		}
		if !plateIn(px+o.x, py+o.y) && plateIn(px, py) {
			return true
		}
	}
	return false
}

// leaf is one silhouette in the foreground canopy.
type leaf struct {
	x, y   float64
	ang    float64
	length float64
	width  float64
}

func (l leaf) coords(px, py float64) (along, across float64) {
	dx, dy := px-l.x, py-l.y
	ca, sa := math.Cos(l.ang), math.Sin(l.ang)
	return dx*ca + dy*sa, -dx*sa + dy*ca
}

func (l leaf) coverage(px, py float64) float64 {
	along, across := l.coords(px, py)
	if along < 0 || along > l.length {
		return 0
	}
	t := along / l.length
	half := l.width * math.Pow(math.Sin(math.Pi*t), 0.72)
	if half <= 0 {
		return 0
	}
	return smooth(1.15, 0.85, math.Abs(across)/half)
}

// rim lights the upper edge of a leaf.
func (l leaf) rim(px, py float64) bool {
	along, across := l.coords(px, py)
	if along < 4 || along > l.length-4 {
		return false
	}
	t := along / l.length
	half := l.width * math.Pow(math.Sin(math.Pi*t), 0.72)
	return half > 0 && math.Abs(across) > half*0.72 && across < 0
}

var leaves = []leaf{
	// bottom-left corner
	{120, 1092, -1.02, 300, 62},
	{196, 1112, -0.58, 250, 52},
	{44, 1064, -1.40, 236, 48},
	{318, 1118, -0.24, 208, 44},
	{246, 1058, -0.86, 186, 38},
	// bottom-right corner
	{1808, 1096, 2.22, 290, 60},
	{1712, 1114, 1.60, 248, 50},
	{1888, 1064, 2.72, 226, 46},
	{1600, 1124, 1.30, 200, 40},
	// hanging vines from the top edge
	{56, -56, 1.34, 220, 38},
	{150, -40, 1.12, 176, 30},
	{1876, -70, 1.86, 210, 36},
	{1786, -46, 2.06, 160, 28},
}
