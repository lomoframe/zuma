package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"zuma/internal/game"
)

// Vertex is the single mesh format used for every solid: position, normal and
// an RGBA colour. The colour is white for anything that is tinted per draw.
type Vertex struct {
	Pos mgl32.Vec3
	Nrm mgl32.Vec3
	Col mgl32.Vec4
	UV  mgl32.Vec2
}

func rgba(r, g, b, a float32) mgl32.Vec4 { return mgl32.Vec4{r, g, b, a} }

// Builder accumulates triangles in board space.
type Builder struct {
	verts []Vertex
	idx   []uint32
}

func (b *Builder) vertex(p, n mgl32.Vec3, c mgl32.Vec4) uint32 {
	b.verts = append(b.verts, Vertex{Pos: p, Nrm: n, Col: c})
	return uint32(len(b.verts) - 1)
}

func (b *Builder) tri(a, c, d uint32) { b.idx = append(b.idx, a, c, d) }

// quad adds two triangles; the caller owns the winding.
func (b *Builder) quad(a, c, d, e uint32) {
	b.idx = append(b.idx, a, c, d, a, d, e)
}

// sphere adds a UV sphere. The triangles wind counter-clockwise seen from
// outside, which the winding test checks.
func (b *Builder) sphere(center mgl32.Vec3, r float32, col mgl32.Vec4, seg, ring int) {
	base := uint32(len(b.verts))
	for j := 0; j <= ring; j++ {
		v := float32(j) / float32(ring)
		theta := v * math.Pi
		st, ct := float32(math.Sin(float64(theta))), float32(math.Cos(float64(theta)))
		for i := 0; i <= seg; i++ {
			phi := float32(i) / float32(seg) * 2 * math.Pi
			n := mgl32.Vec3{st * float32(math.Cos(float64(phi))), ct, st * float32(math.Sin(float64(phi)))}
			b.vertex(center.Add(n.Mul(r)), n, col)
		}
	}
	for j := 0; j < ring; j++ {
		for i := 0; i < seg; i++ {
			a := base + uint32(j*(seg+1)+i)
			c := a + 1
			d := a + uint32(seg+1)
			e := d + 1
			b.tri(a, c, e)
			b.tri(a, e, d)
		}
	}
}

// cylinder adds an open side wall (no caps) around Y.
func (b *Builder) cylinder(cx, cz, r, y0, y1 float32, col mgl32.Vec4, seg int) {
	base := uint32(len(b.verts))
	for i := 0; i <= seg; i++ {
		a := float32(i) / float32(seg) * 2 * math.Pi
		ca, sa := float32(math.Cos(float64(a))), float32(math.Sin(float64(a)))
		n := mgl32.Vec3{ca, 0, sa}
		b.vertex(mgl32.Vec3{cx + r*ca, y0, cz + r*sa}, n, col)
		b.vertex(mgl32.Vec3{cx + r*ca, y1, cz + r*sa}, n, col)
	}
	for i := 0; i < seg; i++ {
		a := base + uint32(i*2)
		c := a + 1
		d := a + 2
		e := a + 3
		b.tri(a, c, e)
		b.tri(a, e, d)
	}
}

// disc adds a flat cap facing up or down.
func (b *Builder) disc(cx, cz, r, y float32, col mgl32.Vec4, seg int, up bool) {
	n := mgl32.Vec3{0, 1, 0}
	if !up {
		n = mgl32.Vec3{0, -1, 0}
	}
	center := b.vertex(mgl32.Vec3{cx, y, cz}, n, col)
	first := uint32(len(b.verts))
	for i := 0; i <= seg; i++ {
		a := float32(i) / float32(seg) * 2 * math.Pi
		p := mgl32.Vec3{cx + r*float32(math.Cos(float64(a))), y, cz + r*float32(math.Sin(float64(a)))}
		b.vertex(p, n, col)
	}
	for i := 0; i < seg; i++ {
		a := first + uint32(i)
		c := a + 1
		if up {
			b.tri(center, c, a)
		} else {
			b.tri(center, a, c)
		}
	}
}

// torus adds a ring lying in the XZ plane.
func (b *Builder) torus(cx, cz, major, minor, y float32, col mgl32.Vec4, segU, segV int) {
	base := uint32(len(b.verts))
	for i := 0; i <= segU; i++ {
		u := float32(i) / float32(segU) * 2 * math.Pi
		cu, su := float32(math.Cos(float64(u))), float32(math.Sin(float64(u)))
		for j := 0; j <= segV; j++ {
			v := float32(j) / float32(segV) * 2 * math.Pi
			cv, sv := float32(math.Cos(float64(v))), float32(math.Sin(float64(v)))
			n := mgl32.Vec3{cv * cu, sv, cv * su}
			p := mgl32.Vec3{cx + (major+minor*cv)*cu, y + minor*sv, cz + (major+minor*cv)*su}
			b.vertex(p, n, col)
		}
	}
	stride := uint32(segV + 1)
	for i := 0; i < segU; i++ {
		for j := 0; j < segV; j++ {
			a := base + uint32(i)*stride + uint32(j)
			c := a + stride
			d := a + 1
			e := c + 1
			b.tri(a, e, c)
			b.tri(a, d, e)
		}
	}
}

// ribbon lays a flat strip of the given width along the path.
func (b *Builder) ribbon(path *game.Path, width, y float32, col mgl32.Vec4) {
	b.ribbonOffset(path, width, 0, y, col)
}

// ribbonOffset lays a strip whose centre line is pushed sideways along the
// path normal, which is how the groove gets its two lit lips.
func (b *Builder) ribbonOffset(path *game.Path, width, offset, y float32, col mgl32.Vec4) {
	const step = 0.02
	n := mgl32.Vec3{0, 1, 0}
	var prevL, prevR uint32
	first := true
	for s := float32(0); s <= path.Len+step*0.5; s += step {
		ss := s
		if ss > path.Len {
			ss = path.Len
		}
		pos, tan := path.At(ss)
		nrm := mgl32.Vec2{tan.Y(), -tan.X()}
		pos = pos.Add(nrm.Mul(offset))
		side := nrm.Mul(width / 2)
		l := b.vertex(mgl32.Vec3{pos.X() - side.X(), y, pos.Y() - side.Y()}, n, col)
		r := b.vertex(mgl32.Vec3{pos.X() + side.X(), y, pos.Y() + side.Y()}, n, col)
		if !first {
			b.quad(prevL, l, r, prevR)
		}
		prevL, prevR = l, r
		first = false
		if ss >= path.Len {
			break
		}
	}
}

// flatQuad adds a single quad from four corners; the normal comes from the
// winding, so the corners must be given counter-clockwise as seen from the
// front face.
func (b *Builder) flatQuad(p0, p1, p2, p3 mgl32.Vec3, col mgl32.Vec4) {
	n := p1.Sub(p0).Cross(p2.Sub(p0))
	if n.Len() > 0 {
		n = n.Normalize()
	}
	a := b.vertex(p0, n, col)
	c := b.vertex(p1, n, col)
	d := b.vertex(p2, n, col)
	e := b.vertex(p3, n, col)
	b.quad(a, c, d, e)
}

// prism extrudes a closed outline between two heights.
func (b *Builder) prism(outline []mgl32.Vec2, y0, y1 float32, top, side mgl32.Vec4) {
	n := len(outline)
	if n < 3 {
		return
	}
	var cx, cz float32
	for _, p := range outline {
		cx += p.X()
		cz += p.Y()
	}
	cx /= float32(n)
	cz /= float32(n)
	ct := b.vertex(mgl32.Vec3{cx, y1, cz}, mgl32.Vec3{0, 1, 0}, top)
	cb := b.vertex(mgl32.Vec3{cx, y0, cz}, mgl32.Vec3{0, -1, 0}, top)
	for i := 0; i < n; i++ {
		p0, p1 := outline[i], outline[(i+1)%n]
		v0 := b.vertex(mgl32.Vec3{p0.X(), y1, p0.Y()}, mgl32.Vec3{0, 1, 0}, top)
		v1 := b.vertex(mgl32.Vec3{p1.X(), y1, p1.Y()}, mgl32.Vec3{0, 1, 0}, top)
		b.tri(ct, v0, v1)
		w0 := b.vertex(mgl32.Vec3{p0.X(), y0, p0.Y()}, mgl32.Vec3{0, -1, 0}, top)
		w1 := b.vertex(mgl32.Vec3{p1.X(), y0, p1.Y()}, mgl32.Vec3{0, -1, 0}, top)
		b.tri(cb, w1, w0)
		d := p1.Sub(p0)
		nrm := mgl32.Vec3{d.Y(), 0, -d.X()}
		if l := nrm.Len(); l > 0 {
			nrm = nrm.Mul(1 / l)
		}
		s0 := b.vertex(mgl32.Vec3{p0.X(), y0, p0.Y()}, nrm, side)
		s1 := b.vertex(mgl32.Vec3{p1.X(), y0, p1.Y()}, nrm, side)
		s2 := b.vertex(mgl32.Vec3{p1.X(), y1, p1.Y()}, nrm, side)
		s3 := b.vertex(mgl32.Vec3{p0.X(), y1, p0.Y()}, nrm, side)
		b.quad(s0, s1, s2, s3)
	}
}

// face fills a closed outline at one height, visible from both sides.
func (b *Builder) face(outline []mgl32.Vec2, y float32, col mgl32.Vec4) {
	n := len(outline)
	if n < 3 {
		return
	}
	var cx, cz float32
	for _, p := range outline {
		cx += p.X()
		cz += p.Y()
	}
	cx /= float32(n)
	cz /= float32(n)
	for _, up := range []float32{1, -1} {
		c := b.vertex(mgl32.Vec3{cx, y, cz}, mgl32.Vec3{0, up, 0}, col)
		for i := 0; i < n; i++ {
			p0, p1 := outline[i], outline[(i+1)%n]
			v0 := b.vertex(mgl32.Vec3{p0.X(), y, p0.Y()}, mgl32.Vec3{0, up, 0}, col)
			v1 := b.vertex(mgl32.Vec3{p1.X(), y, p1.Y()}, mgl32.Vec3{0, up, 0}, col)
			if up > 0 {
				b.tri(c, v0, v1)
			} else {
				b.tri(c, v1, v0)
			}
		}
	}
}
