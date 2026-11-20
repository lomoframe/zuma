package render

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"

	"zuma/internal/font"
	"zuma/internal/game"
	"zuma/internal/gl"
)

const (
	blendSrcAlpha         = uint32(0x0302)
	blendOneMinusSrcAlpha = uint32(0x0303)
)

var debugText = false

// SetTextDebug prints the rasterised line sizes, which is the quickest way to
// see why a HUD line lands in the wrong place.
func SetTextDebug(on bool) { debugText = on }

const meshVS = `#version 410 core
layout(location=0) in vec3 aPos;
layout(location=1) in vec3 aNrm;
layout(location=2) in vec4 aCol;
uniform mat4 uVP;
uniform mat4 uModel;
out vec3 vNrm;
out vec3 vWorld;
out vec4 vCol;
out vec3 vLocal;
void main() {
        vLocal = aPos;
        vec4 w = uModel * vec4(aPos, 1.0);
        vWorld = w.xyz;
        vNrm = mat3(uModel) * aNrm;
        vCol = aCol;
        gl_Position = uVP * w;
}`

const meshFS = `#version 410 core
in vec3 vNrm;
in vec3 vWorld;
in vec4 vCol;
in vec3 vLocal;
uniform mat3 uBallRot;
uniform float uBall;
uniform vec4 uTint;
uniform vec3 uCam;
uniform float uGloss;
uniform float uUnlit;
out vec4 frag;

float hash13(vec3 p) {
        p = fract(p * 0.3183099 + vec3(0.11, 0.17, 0.23));
        p *= 17.0;
        return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}

void main() {
        vec3 n = normalize(vNrm);
        vec3 L = normalize(vec3(0.38, 0.88, 0.30));
        vec3 base = vCol.rgb * uTint.rgb;
        if (uBall > 0.5) {
                // Fine speckle noise only: no stripes. It rotates with the
                // ball's real rolling, so the grain visibly flows.
                vec3 p = uBallRot * vLocal;
                float n1 = hash13(floor(p * 11.0));
                float n2 = hash13(floor(p * 27.0 + 7.0));
                float n3 = hash13(floor(p * 53.0 + 19.0));
                base *= 0.84 + 0.13 * n1 + 0.07 * n2 + 0.04 * n3;
        }
        if (uUnlit > 0.5) {
                frag = vec4(base, vCol.a * uTint.a);
                return;
        }
        float d = max(dot(n, L), 0.0);
        vec3 amb = vec3(0.30, 0.37, 0.48);
        vec3 col = base * (amb + d * vec3(1.06, 0.99, 0.88) * 0.82);
        vec3 V = normalize(uCam - vWorld);
        vec3 H = normalize(L + V);
        col += vec3(1.0, 0.97, 0.92) * pow(max(dot(n, H), 0.0), 64.0) * uGloss;
        float rim = pow(1.0 - max(dot(n, V), 0.0), 3.0);
        col += base * rim * 0.20;
        frag = vec4(col, vCol.a * uTint.a);
}`

const textVS = `#version 410 core
layout(location=0) in vec3 aPos;
layout(location=1) in vec2 aUV;
uniform mat4 uMVP;
out vec2 vUV;
void main() {
        vUV = aUV;
        gl_Position = uMVP * vec4(aPos, 1.0);
}`

const textFS = `#version 410 core
in vec2 vUV;
uniform sampler2D uTex;
uniform vec4 uColor;
out vec4 frag;
void main() {
        float a = texture(uTex, vUV).a;
        frag = vec4(uColor.rgb, a);
}`

// Mesh is an uploaded triangle soup.
type Mesh struct {
	vao, vbo, ebo uint32
	count         int32
}

func upload(b *Builder) *Mesh {
	var buf []byte
	for _, v := range b.verts {
		for i := 0; i < 3; i++ {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(v.Pos[i]))
		}
		for i := 0; i < 3; i++ {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(v.Nrm[i]))
		}
		for i := 0; i < 4; i++ {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(v.Col[i]))
		}
	}
	var ibuf []byte
	for _, i := range b.idx {
		ibuf = binary.LittleEndian.AppendUint32(ibuf, i)
	}

	m := &Mesh{vao: gl.GenVAO(), vbo: gl.GenBuffer(), ebo: gl.GenBuffer(), count: int32(len(b.idx))}
	gl.BindVAO(m.vao)
	gl.BindBuffer(gl.ArrayBuffer, m.vbo)
	gl.BufferData(gl.ArrayBuffer, buf, gl.StaticDraw)
	gl.BindBuffer(gl.ElementArrayBuffer, m.ebo)
	gl.BufferData(gl.ElementArrayBuffer, ibuf, gl.StaticDraw)
	const stride = int32(10 * 4)
	gl.EnableAttrib(0)
	gl.AttribPtr(0, 3, gl.Float, false, stride, 0)
	gl.EnableAttrib(1)
	gl.AttribPtr(1, 3, gl.Float, false, stride, 12)
	gl.EnableAttrib(2)
	gl.AttribPtr(2, 4, gl.Float, false, stride, 24)
	gl.BindVAO(0)
	return m
}

func (m *Mesh) draw() {
	if m == nil || m.count == 0 {
		return
	}
	gl.BindVAO(m.vao)
	gl.DrawElements(gl.Triangles, m.count, gl.UnsignedInt)
}

// textQuad is the four-corner quad the text shader draws. Its vertices are
// uploaded in world space each line, which keeps the placement explicit and
// works the same in the desktop view and inside the headset.
type textQuad struct {
	vao, vbo uint32
}

func newTextQuad() *textQuad {
	idx := []uint32{0, 1, 2, 0, 2, 3}
	var ibuf []byte
	for _, i := range idx {
		ibuf = binary.LittleEndian.AppendUint32(ibuf, i)
	}
	q := &textQuad{vao: gl.GenVAO(), vbo: gl.GenBuffer()}
	gl.BindVAO(q.vao)
	gl.BindBuffer(gl.ArrayBuffer, q.vbo)
	gl.BufferData(gl.ArrayBuffer, make([]byte, 4*5*4), gl.DynamicDraw)
	ebo := gl.GenBuffer()
	gl.BindBuffer(gl.ElementArrayBuffer, ebo)
	gl.BufferData(gl.ElementArrayBuffer, ibuf, gl.StaticDraw)
	gl.EnableAttrib(0)
	gl.AttribPtr(0, 3, gl.Float, false, 20, 0)
	gl.EnableAttrib(1)
	gl.AttribPtr(1, 2, gl.Float, false, 20, 12)
	gl.BindVAO(0)
	return q
}

// upload replaces the four corners; UVs are fixed, with v=0 at the top row of
// the rasterised mask.
func (q *textQuad) upload(c [4]mgl32.Vec3) {
	uv := [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}
	var buf []byte
	for i := 0; i < 4; i++ {
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(c[i][0]))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(c[i][1]))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(c[i][2]))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(uv[i][0]))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(uv[i][1]))
	}
	gl.BindBuffer(gl.ArrayBuffer, q.vbo)
	gl.BufferData(gl.ArrayBuffer, buf, gl.DynamicDraw)
}

func (q *textQuad) draw() {
	gl.BindVAO(q.vao)
	gl.DrawElements(gl.Triangles, 6, gl.UnsignedInt)
}

type textTex struct {
	tex  uint32
	w, h int
}

// Renderer draws the whole board.
type Renderer struct {
	meshProg gl.Program
	textProg gl.Program

	uVP, uModel, uTint, uCam, uGloss, uUnlit, uBall int32
	uBallRot                                        mat3Loc
	uMVP, uTex, uColor                              int32

	board *Mesh
	// rim is the painted edge band, drawn with the colour of the bead in hand.
	rim      *Mesh
	table    *Mesh
	path     *game.Path
	pathKey  string
	frog     *Mesh
	ball     *Mesh
	ring     *Mesh
	beam     *Mesh
	seam     *Mesh
	hand     *Mesh
	handName string
	quad     *textQuad

	// Loading screen.
	splashProg             gl.Program
	uSplashMVP, uSplashTex int32
	uSplashTint            int32
	splashBG               uint32
	splashBGQuad           *textQuad
	splash                 *splashState

	texts map[string]*textTex
}

// mat3Loc is just a uniform location, named to keep the struct readable.
type mat3Loc = int32

func rotAxis(axis mgl32.Vec3, angle float32) mgl32.Mat3 {
	a := axis.Normalize()
	x, y, z := a[0], a[1], a[2]
	c, sn := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))
	t := 1 - c
	// Row-major entries, then stored column-major.
	return mgl32.Mat3{
		t*x*x + c, t*x*y + sn*z, t*x*z - sn*y,
		t*x*y - sn*z, t*y*y + c, t*y*z + sn*x,
		t*x*z + sn*y, t*y*z - sn*x, t*z*z + c,
	}
}

func mat3Mul(a, b mgl32.Mat3) mgl32.Mat3 {
	var r mgl32.Mat3
	for col := 0; col < 3; col++ {
		for row := 0; row < 3; row++ {
			r[col*3+row] = a[0*3+row]*b[col*3+0] + a[1*3+row]*b[col*3+1] + a[2*3+row]*b[col*3+2]
		}
	}
	return r
}

// ballPhase decorrelates the grain of neighbouring beads without touching
// the axis they roll about.
func ballPhase(id int) float32 {
	h := uint32(id)*2246822519 + 0x165667b1
	return float32(h%62832) / 10000 // 0 .. 2π
}

// randomBallRot gives every bead its own texture orientation so the seams do
// not line up across the chain.
func randomBallRot(id int) mgl32.Mat3 {
	h := uint32(id)*2654435761 + 0x9e3779b9
	ax := mgl32.Vec3{
		float32(h%1000)/1000 - 0.5,
		float32((h>>10)%1000)/1000 - 0.5,
		float32((h>>20)%1000)/1000 - 0.5,
	}
	if ax.Len() < 0.1 {
		ax = mgl32.Vec3{0.3, 0.8, 0.2}
	}
	return rotAxis(ax, float32(h%628)/100)
}

// DrawStatic draws the table, which does not tilt with the playfield.
func (r *Renderer) DrawStatic(vp, world mgl32.Mat4) {
	r.meshProg.Bind()
	r.meshProg.Mat4(r.uVP, vp)
	r.meshProg.Mat4(r.uModel, world)
	r.meshProg.Vec4(r.uTint, 1, 1, 1, 1)
	r.meshProg.Float(r.uGloss, 0.25)
	r.meshProg.Float(r.uUnlit, 0)
	r.meshProg.Float(r.uBall, 0)
	r.table.draw()
}

// SetPath rebuilds the board geometry when a level's spiral changes. The old
// mesh is left to the driver; a session only builds a handful of boards.
func (r *Renderer) SetPath(bd game.Board, p *game.Path) {
	key := p.Key + "|" + fmt.Sprintf("%.2f/%.2f/%.2f", bd.W, bd.H, bd.Corner)
	if r.path != nil && r.pathKey == key {
		return
	}
	r.board = upload(buildBoard(bd, p))
	r.rim = upload(buildRim(bd))
	r.table = upload(buildTable(bd))
	r.path = p
	r.pathKey = key
}

// New compiles the shaders and uploads the static geometry.
func New() (*Renderer, error) {
	mp, err := gl.NewProgram(meshVS, meshFS)
	if err != nil {
		return nil, fmt.Errorf("mesh shader: %w", err)
	}
	tp, err := gl.NewProgram(textVS, textFS)
	if err != nil {
		return nil, fmt.Errorf("text shader: %w", err)
	}
	r := &Renderer{
		meshProg: mp,
		textProg: tp,
		uVP:      mp.Loc("uVP"),
		uModel:   mp.Loc("uModel"),
		uTint:    mp.Loc("uTint"),
		uCam:     mp.Loc("uCam"),
		uGloss:   mp.Loc("uGloss"),
		uUnlit:   mp.Loc("uUnlit"),
		uBall:    mp.Loc("uBall"),
		uBallRot: mat3Loc(mp.Loc("uBallRot")),
		uMVP:     tp.Loc("uMVP"),
		uTex:     tp.Loc("uTex"),
		uColor:   tp.Loc("uColor"),
		texts:    map[string]*textTex{},
	}
	r.SetPath(game.CircleBoard(3.10), game.NewPathForBoard())
	r.frog = upload(buildFrog())
	r.ball = upload(buildBall())
	r.ring = upload(buildRing())
	r.beam = upload(buildBeam())
	r.seam = upload(buildSeam())
	r.quad = newTextQuad()
	if err := r.newSplashGeometry(); err != nil {
		return nil, err
	}
	return r, nil
}

func buildBall() *Builder {
	b := &Builder{}
	b.sphere(mgl32.Vec3{}, 1, rgba(1, 1, 1, 1), 26, 14)
	return b
}

// buildBeam is a unit cylinder along +Z, centred on the origin: scale it to a
// length and orient it between the hand and the aim point.
func buildBeam() *Builder {
	b := &Builder{}
	const seg = 10
	base := uint32(len(b.verts))
	for i := 0; i <= seg; i++ {
		a := float32(i) / seg * 2 * math.Pi
		ca, sa := float32(math.Cos(float64(a))), float32(math.Sin(float64(a)))
		n := mgl32.Vec3{ca, sa, 0}
		b.vertex(mgl32.Vec3{ca, sa, -0.5}, n, rgba(1, 1, 1, 1))
		b.vertex(mgl32.Vec3{ca, sa, 0.5}, n, rgba(1, 1, 1, 1))
	}
	for i := 0; i < seg; i++ {
		a := base + uint32(i*2)
		c := a + 1
		d := a + 2
		e := a + 3
		b.tri(a, e, c)
		b.tri(a, d, e)
	}
	return b
}

// buildSeam is a flat unit quad in XZ: x across the groove, z along it.
func buildSeam() *Builder {
	b := &Builder{}
	n := mgl32.Vec3{0, 1, 0}
	a := b.vertex(mgl32.Vec3{-0.5, 0, -0.5}, n, rgba(1, 1, 1, 1))
	c := b.vertex(mgl32.Vec3{0.5, 0, -0.5}, n, rgba(1, 1, 1, 1))
	d := b.vertex(mgl32.Vec3{0.5, 0, 0.5}, n, rgba(1, 1, 1, 1))
	e := b.vertex(mgl32.Vec3{-0.5, 0, 0.5}, n, rgba(1, 1, 1, 1))
	b.quad(a, c, d, e)
	return b
}

func buildRing() *Builder {
	b := &Builder{}
	b.torus(0, 0, 1, 0.22, 0, rgba(1, 1, 1, 1), 48, 8)
	return b
}

// buildTable is the table the playfield sits on. It is drawn upright even when
// the playfield itself is tilted.
func tableRadius(bd game.Board) float32 {
	r := bd.HalfW()
	if bd.HalfH() > r {
		r = bd.HalfH()
	}
	return r + 0.55 // the felt edge plus the wooden rim, with a margin
}

// buildRim is the painted band around the felt. Its vertices are white so the
// draw call can tint it with whatever bead the player is holding.
func buildRim(bd game.Board) *Builder {
	b := &Builder{}
	pts := bd.Inflate(0.115).Outline(180)
	if len(pts) > 0 {
		pts = append(pts, pts[0])
	}
	b.ribbon(game.NewPath(pts), 0.035, 0.020, rgba(1, 1, 1, 1))
	return b
}

func buildTable(bd game.Board) *Builder {
	b := &Builder{}
	const seg = 120
	table := rgba(0.115, 0.080, 0.062, 1)
	r := tableRadius(bd)
	b.disc(0, 0, r, -0.135, rgba(0.05, 0.035, 0.028, 1), seg, false)
	b.cylinder(0, 0, r, -0.135, -0.10, table, seg)
	b.disc(0, 0, r, -0.10, table, seg, true)
	return b
}

// buildBoard lays out the playfield: wooden disc, felt, the spiral groove, the
// entry mouth, the hole and the score plaque.
func buildBoard(bd game.Board, path *game.Path) *Builder {
	b := &Builder{}
	wood := rgba(0.22, 0.14, 0.10, 1)
	woodDark := rgba(0.10, 0.065, 0.05, 1)
	felt := rgba(0.105, 0.300, 0.305, 1)
	feltHi := rgba(0.14, 0.36, 0.36, 1)
	stone := rgba(0.22, 0.23, 0.26, 1)

	// The felt takes the board's own silhouette, so oval / rounded-rectangle /
	// strip boards are extruded from their outline instead of being discs.
	outer := bd.Inflate(0.17)
	b.prism(outer.Outline(140), -0.10, 0.0, woodDark, wood)
	b.face(bd.Outline(140), 0.005, felt)
	closeLoop := func(pts []mgl32.Vec2) []mgl32.Vec2 {
		if len(pts) > 0 {
			pts = append(pts, pts[0])
		}
		return pts
	}
	b.ribbon(game.NewPath(closeLoop(bd.Inflate(0.01).Outline(180))), 0.05, 0.013, feltHi)

	// The groove: a channel just wide enough for the beads, with two lit lips.
	b.ribbon(path, game.Diam*1.30, 0.0120, rgba(0.030, 0.075, 0.078, 1))
	b.ribbon(path, game.Diam*1.10, 0.0128, rgba(0.004, 0.010, 0.012, 1))
	b.ribbonOffset(path, game.Diam*0.22, game.Diam*0.80, 0.0140, rgba(0.20, 0.46, 0.45, 1))
	b.ribbonOffset(path, game.Diam*0.22, -game.Diam*0.80, 0.0140, rgba(0.20, 0.46, 0.45, 1))

	// Entry mouth at s=0.
	pos, _ := path.At(0)
	b.disc(pos.X(), pos.Y(), 0.095, 0.016, rgba(0, 0, 0, 1), 24, true)
	b.torus(pos.X(), pos.Y(), 0.108, 0.032, 0.030, stone, 24, 8)

	// Hole at the end of the path.
	end, _ := path.At(path.Len)
	b.disc(end.X(), end.Y(), 0.155, 0.014, rgba(0.004, 0.005, 0.007, 1), 40, true)
	b.torus(end.X(), end.Y(), 0.168, 0.038, 0.030, stone, 40, 10)
	b.sphere(mgl32.Vec3{end.X() - 0.058, 0.064, end.Y() - 0.120}, 0.021, rgba(1.0, 0.14, 0.09, 1), 12, 8)
	b.sphere(mgl32.Vec3{end.X() + 0.058, 0.064, end.Y() - 0.120}, 0.021, rgba(1.0, 0.14, 0.09, 1), 12, 8)

	return b
}

// buildFrog builds the turret in local space, facing +Z.
func buildFrog() *Builder {
	b := &Builder{}
	stone := rgba(0.17, 0.18, 0.21, 1)
	stoneHi := rgba(0.27, 0.29, 0.33, 1)
	gold := rgba(0.74, 0.52, 0.18, 1)
	skin := rgba(0.26, 0.64, 0.27, 1)
	skinDark := rgba(0.15, 0.44, 0.18, 1)
	cream := rgba(0.95, 0.91, 0.72, 1)
	black := rgba(0.02, 0.02, 0.03, 1)

	b.cylinder(0, 0, 0.175, 0.0, 0.055, stone, 32)
	b.disc(0, 0, 0.175, 0.055, stoneHi, 32, true)
	b.torus(0, 0, 0.175, 0.018, 0.052, gold, 32, 8)

	b.sphere(mgl32.Vec3{0, 0.145, 0.0}, 0.150, skin, 28, 16)
	b.sphere(mgl32.Vec3{0, 0.115, 0.075}, 0.102, cream, 24, 14)
	b.sphere(mgl32.Vec3{-0.125, 0.085, -0.02}, 0.062, skinDark, 18, 12)
	b.sphere(mgl32.Vec3{0.125, 0.085, -0.02}, 0.062, skinDark, 18, 12)

	b.sphere(mgl32.Vec3{-0.080, 0.272, 0.030}, 0.052, cream, 18, 12)
	b.sphere(mgl32.Vec3{0.080, 0.272, 0.030}, 0.052, cream, 18, 12)
	b.sphere(mgl32.Vec3{-0.080, 0.315, 0.050}, 0.020, black, 12, 8)
	b.sphere(mgl32.Vec3{0.080, 0.315, 0.050}, 0.020, black, 12, 8)

	b.sphere(mgl32.Vec3{0, 0.085, 0.115}, 0.056, skin, 20, 12)
	b.flatQuad(mgl32.Vec3{-0.044, 0.040, 0.172}, mgl32.Vec3{0.044, 0.040, 0.172},
		mgl32.Vec3{0.044, 0.122, 0.172}, mgl32.Vec3{-0.044, 0.122, 0.172}, black)
	return b
}

func ballColor(i int) mgl32.Vec4 {
	if i < 0 || i >= len(game.Palette) {
		i = 0
	}
	c := game.Palette[i]
	return mgl32.Vec4{c.R, c.G, c.B, 1}
}

// Draw renders one frame. vp is in world space; world maps board space into
// the world (identity for the desktop camera).
// SetControllerModel uploads SteamVR's own controller mesh, in metres in the
// device's local space.
func (r *Renderer) SetControllerModel(pos, nrm []float32, idx []uint16, name string) {
	n := len(pos) / 3
	if n == 0 || len(nrm) < n*3 || len(idx) == 0 {
		return
	}
	b := &Builder{}
	for i := 0; i < n; i++ {
		b.verts = append(b.verts, Vertex{
			Pos: mgl32.Vec3{pos[i*3], pos[i*3+1], pos[i*3+2]},
			Nrm: mgl32.Vec3{nrm[i*3], nrm[i*3+1], nrm[i*3+2]},
			Col: rgba(1, 1, 1, 1),
		})
	}
	for _, i := range idx {
		b.idx = append(b.idx, uint32(i))
	}
	r.hand = upload(b)
	r.handName = name
}

// drawController draws SteamVR's controller model at the device pose plus the
// device axes, so the physical pad can be compared with what the runtime says.
func (r *Renderer) drawController(pose mgl32.Mat4) {
	if r.hand != nil {
		r.meshProg.Mat4(r.uModel, pose)
		r.meshProg.Vec4(r.uTint, 0.62, 0.66, 0.74, 1)
		r.meshProg.Float(r.uGloss, 0.30)
		r.meshProg.Float(r.uUnlit, 0)
		r.hand.draw()
	}
	o := mgl32.Vec3{pose[12], pose[13], pose[14]}
	axes := [3]mgl32.Vec3{
		{pose[0], pose[1], pose[2]},
		{pose[4], pose[5], pose[6]},
		{pose[8], pose[9], pose[10]},
	}
	cols := [3]mgl32.Vec4{rgba(1, 0.25, 0.25, 1), rgba(0.3, 1, 0.35, 1), rgba(0.35, 0.55, 1, 1)}
	for i := 0; i < 3; i++ {
		d := axes[i].Normalize()
		r.drawBeamColored(mgl32.Ident4(), o, o.Add(d.Mul(0.09)), cols[i], 0.004)
	}
}

// Draw renders the round. world places the table (it carries the tilt); hud is
// the same frame without that tilt, so the score and result text always stand
// upright in the room.
func (r *Renderer) Draw(vp mgl32.Mat4, cam mgl32.Vec3, world, hud mgl32.Mat4, g *game.Game, now float64, aimO, aimD mgl32.Vec3, aimOK bool, hand mgl32.Mat4, handOK bool) {
	r.meshProg.Bind()
	r.meshProg.Mat4(r.uVP, vp)
	r.meshProg.Vec3(r.uCam, cam[0], cam[1], cam[2])
	gl.Enable(gl.DepthTest)
	gl.Enable(gl.CullFace)
	gl.CullFaceMode(gl.Back)

	r.meshProg.Float(r.uBall, 0)
	r.meshProg.Mat3(r.uBallRot, mgl32.Ident3())
	r.meshProg.Mat4(r.uModel, world)
	r.meshProg.Vec4(r.uTint, 1, 1, 1, 1)
	r.meshProg.Float(r.uGloss, 0.25)
	r.board.draw()

	// The rim band wears the colour of the bead in hand.
	if r.rim != nil {
		c := ballColor(g.Loaded)
		r.meshProg.Mat4(r.uModel, world)
		r.meshProg.Vec4(r.uTint, c[0], c[1], c[2], 1)
		r.meshProg.Float(r.uGloss, 0.25)
		r.rim.draw()
		r.meshProg.Vec4(r.uTint, 1, 1, 1, 1)
	}

	// Chain beads.
	r.meshProg.Float(r.uGloss, 1.10)
	for i, bd := range g.Balls {
		s := bd.S
		if g.PushFrom() >= 0 && i >= g.PushFrom() {
			s -= g.Push()
		}
		pos := g.Path.PointAt(s)
		radius := game.BallR
		if bd.InsertAnim > 0 {
			// Slide the fresh bead from the point it struck into its slot.
			t := 1 - bd.InsertAnim/game.InsertAnimTime
			t = 1 - (1-t)*(1-t)
			pos = bd.From.Add(pos.Sub(bd.From).Mul(t))
			radius = game.BallR * (1 + 0.35*(1-t))
		}
		model := world.
			Mul4(mgl32.Translate3D(pos.X(), game.BallR, pos.Y())).
			Mul4(mgl32.Scale3D(radius, radius, radius))
		c := ballColor(bd.Color)
		r.meshProg.Mat4(r.uModel, model)
		r.meshProg.Vec4(r.uTint, c[0], c[1], c[2], 1)
		_, tan := g.Path.At(bd.S)
		side := mgl32.Vec3{tan.Y(), 0, -tan.X()} // perpendicular to travel, horizontal
		// One shared axis for the whole chain: the bead turns about the
		// horizontal axis across its travel. Beads differ only by a phase
		// offset, so the grain reads as one surface rolling forward instead
		// of every bead spinning in its own direction.
		roll := rotAxis(side, bd.S/game.BallR+ballPhase(bd.ID))
		r.meshProg.Mat3(r.uBallRot, roll)
		r.meshProg.Float(r.uBall, 1)
		r.meshProg.Float(r.uGloss, 0.0)
		r.ball.draw()
		r.meshProg.Float(r.uBall, 0)
	}

	// Balls in the air.
	for _, sh := range g.Shots {
		// The shot lives in 3D: draw it at its real height, not on the board.
		model := world.
			Mul4(mgl32.Translate3D(sh.Pos.X(), sh.Pos.Y(), sh.Pos.Z())).
			Mul4(mgl32.Scale3D(game.BallR, game.BallR, game.BallR))
		c := ballColor(sh.Color)
		r.meshProg.Mat4(r.uModel, model)
		r.meshProg.Vec4(r.uTint, c[0], c[1], c[2], 1)
		r.meshProg.Float(r.uBall, 1)
		r.meshProg.Float(r.uGloss, 0.0)
		r.meshProg.Mat3(r.uBallRot, mat3Mul(rotAxis(mgl32.Vec3{0.4, 1, 0.2}, float32(now)*18), randomBallRot(sh.Color+7)))
		r.ball.draw()
		r.meshProg.Float(r.uBall, 0)
	}

	if handOK {
		r.drawController(hand)
	}
	// The player is the launcher: a beam leaves the controller and a bead
	// waits at its tip.
	if aimOK {
		r.drawHand(world, g, aimO, aimD, now)
	}

	r.drawHUD(vp, hud, g, now)
}

func (r *Renderer) drawDot(world mgl32.Mat4, pos mgl32.Vec2, radius float32) {
	model := world.
		Mul4(mgl32.Translate3D(pos.X(), 0.016+radius, pos.Y())).
		Mul4(mgl32.Scale3D(radius, radius, radius))
	r.meshProg.Mat4(r.uModel, model)
	r.meshProg.Vec4(r.uTint, 0.55, 0.88, 1.0, 1)
	r.meshProg.Float(r.uGloss, 0)
	r.ball.draw()
}

func (r *Renderer) drawBallAt(world mgl32.Mat4, pos mgl32.Vec2, radius float32, color int, phase float64) {
	r.drawBallAtHeight(world, pos, radius, radius, color, phase)
}

func (r *Renderer) drawBallAtHeight(world mgl32.Mat4, pos mgl32.Vec2, height, radius float32, color int, phase float64) {
	bob := float32(math.Sin(phase*4.0)) * radius * 0.10
	model := world.
		Mul4(mgl32.Translate3D(pos.X(), height+bob, pos.Y())).
		Mul4(mgl32.Scale3D(radius, radius, radius))
	c := ballColor(color)
	r.meshProg.Mat4(r.uModel, model)
	r.meshProg.Vec4(r.uTint, c[0], c[1], c[2], 1)
	r.meshProg.Float(r.uGloss, 1.2)
	r.ball.draw()
}

func (r *Renderer) drawRing(world mgl32.Mat4, at mgl32.Vec2, radius float32, col mgl32.Vec4) {
	model := world.
		Mul4(mgl32.Translate3D(at.X(), 0.019, at.Y())).
		Mul4(mgl32.Scale3D(radius, radius, radius))
	r.meshProg.Mat4(r.uModel, model)
	r.meshProg.Vec4(r.uTint, col[0], col[1], col[2], col[3])
	r.meshProg.Float(r.uGloss, 0)
	r.ring.draw()
}

// seamMark is the gap a shot would be inserted into.
type seamMark struct {
	pos mgl32.Vec3
	tan mgl32.Vec2
}

// aimTarget returns where the beam ends and, when it meets the chain, the seam
// between the two beads the shot would be inserted between.
func (r *Renderer) aimTarget(g *game.Game, o, d mgl32.Vec3) (mgl32.Vec3, seamMark, bool) {
	bestT := float32(1e9)
	bestIdx := -1
	var hit mgl32.Vec3
	for i, b := range g.Balls {
		q := g.Path.PointAt(b.S)
		c := mgl32.Vec3{q.X(), game.BallR, q.Y()}
		oc := o.Sub(c)
		bq := oc.Dot(d)
		cq := oc.LenSqr() - 0.10*0.10
		disc := bq*bq - cq
		if disc <= 0 {
			continue
		}
		t := -bq - float32(math.Sqrt(float64(disc)))
		if t > 0.03 && t < bestT {
			bestT = t
			bestIdx = i
			hit = o.Add(d.Mul(t))
		}
	}
	if bestIdx < 0 {
		end := o.Add(d.Mul(2.6))
		if d.Y() < -0.01 {
			if t := (0.02 - o.Y()) / d.Y(); t > 0.03 && t < 3.0 {
				end = o.Add(d.Mul(t))
			}
		}
		return end, seamMark{}, false
	}

	s := g.Balls[bestIdx].S
	ball := g.Path.PointAt(s)
	_, tan := g.Path.At(s)
	ahead := (mgl32.Vec2{hit.X() - ball.X(), hit.Z() - ball.Y()}).Dot(tan) > 0
	seamS := s + game.Diam*0.5
	if ahead {
		seamS = s - game.Diam*0.5
		if bestIdx > 0 {
			seamS = (s + g.Balls[bestIdx-1].S) * 0.5
		}
	} else if bestIdx+1 < len(g.Balls) {
		seamS = (s + g.Balls[bestIdx+1].S) * 0.5
	}
	p := g.Path.PointAt(seamS)
	seam := mgl32.Vec3{p.X(), game.BallR, p.Y()}
	return hit, seamMark{pos: seam, tan: tan}, true
}

// drawSeam draws a bright band across the groove at the insertion gap.
func (r *Renderer) drawSeam(world mgl32.Mat4, m seamMark) {
	yaw := float32(math.Atan2(float64(m.tan.X()), float64(m.tan.Y())))
	model := world.
		Mul4(mgl32.Translate3D(m.pos.X(), game.BallR+0.004, m.pos.Z())).
		Mul4(mgl32.HomogRotate3DY(yaw)).
		Mul4(mgl32.Scale3D(game.Diam*1.9, 1, game.Diam*0.16))
	r.meshProg.Mat4(r.uModel, model)
	r.meshProg.Vec4(r.uTint, 0.55, 1.0, 1.0, 0.95)
	r.meshProg.Float(r.uGloss, 0)
	r.meshProg.Float(r.uUnlit, 1)
	r.seam.draw()
	r.meshProg.Float(r.uUnlit, 0)
}

// drawHand draws the aiming beam, its reticle, and the loaded and next beads
// floating at the controller.
func (r *Renderer) drawHand(world mgl32.Mat4, g *game.Game, o, d mgl32.Vec3, now float64) {
	end, _, _ := r.aimTarget(g, o, d)
	r.drawBeam(world, o, end)

	// Only the loaded bead rides on the controller.
	r.drawBallWorld(world, o.Add(d.Mul(0.10)), game.BallR, g.Loaded, now)
}

// drawBeam stretches the unit beam between two board-space points.
func (r *Renderer) drawBeam(world mgl32.Mat4, from, to mgl32.Vec3) {
	r.drawBeamColored(world, from, to, rgba(0.40, 0.95, 1.0, 0.95), 0.006)
}

func (r *Renderer) drawBeamColored(world mgl32.Mat4, from, to mgl32.Vec3, col mgl32.Vec4, radius float32) {
	d := to.Sub(from)
	l := d.Len()
	if l < 1e-4 {
		return
	}
	z := d.Mul(1 / l)
	up := mgl32.Vec3{0, 1, 0}
	if math.Abs(float64(z.Y())) > 0.95 {
		up = mgl32.Vec3{1, 0, 0}
	}
	x := up.Cross(z).Normalize()
	y := z.Cross(x)
	c := from.Add(d.Mul(0.5))
	m := mgl32.Mat4{
		x.X(), x.Y(), x.Z(), 0,
		y.X(), y.Y(), y.Z(), 0,
		z.X(), z.Y(), z.Z(), 0,
		c.X(), c.Y(), c.Z(), 1,
	}
	model := world.Mul4(m).Mul4(mgl32.Scale3D(radius, radius, l))
	r.meshProg.Mat4(r.uModel, model)
	r.meshProg.Vec4(r.uTint, col[0], col[1], col[2], col[3])
	r.meshProg.Float(r.uGloss, 0)
	r.meshProg.Float(r.uUnlit, 1)
	r.beam.draw()
	r.meshProg.Float(r.uUnlit, 0)
}

func (r *Renderer) drawRingAt(world mgl32.Mat4, at mgl32.Vec3, radius float32, col mgl32.Vec4) {
	model := world.
		Mul4(mgl32.Translate3D(at.X(), at.Y()+0.003, at.Z())).
		Mul4(mgl32.Scale3D(radius, radius, radius))
	r.meshProg.Mat4(r.uModel, model)
	r.meshProg.Vec4(r.uTint, col[0], col[1], col[2], col[3])
	r.meshProg.Float(r.uGloss, 0)
	r.meshProg.Float(r.uUnlit, 1)
	r.ring.draw()
	r.meshProg.Float(r.uUnlit, 0)
}

func (r *Renderer) drawBallWorld(world mgl32.Mat4, at mgl32.Vec3, radius float32, color int, phase float64) {
	bob := float32(math.Sin(phase*4.0)) * radius * 0.12
	model := world.
		Mul4(mgl32.Translate3D(at.X(), at.Y()+bob, at.Z())).
		Mul4(mgl32.Scale3D(radius, radius, radius))
	c := ballColor(color)
	r.meshProg.Mat4(r.uModel, model)
	r.meshProg.Vec4(r.uTint, c[0], c[1], c[2], 1)
	r.meshProg.Float(r.uGloss, 0.0)
	r.meshProg.Float(r.uUnlit, 0)
	r.meshProg.Float(r.uBall, 1)
	r.meshProg.Mat3(r.uBallRot, mat3Mul(rotAxis(mgl32.Vec3{0.3, 1, 0.1}, float32(phase)*0.8), randomBallRot(color+13)))
	r.ball.draw()
	r.meshProg.Float(r.uBall, 0)
}

// drawHUD lays the score plaque above the far rim.
func (r *Renderer) drawHUD(vp mgl32.Mat4, hud mgl32.Mat4, g *game.Game, now float64) {
	base := hud.Mul4(mgl32.Translate3D(0, 0, -1.88))
	scoreCol := rgba(1, 1, 1, 0.97)
	if f := g.Flash; f > 0 {
		scoreCol = rgba(1, 1-0.15*f, 1-0.55*f, 0.97)
	}
	r.drawText(vp, base, -0.74, 1.15, 46, fmt.Sprintf("分数 %d   第 %d 关", g.Score, g.Level), alignLeft, scoreCol)
	line2 := ""
	switch {
	case g.State == game.StateLost && g.LoseReady():
		line2 = "按 R 或扳机 重新开始"
	case g.State == game.StatePlaying && g.Score == 0:
		line2 = "左键 / 扳机 发射"
	case g.Combo > 1:
		line2 = fmt.Sprintf("本发连击 x%d   剩余 %d", g.Combo, len(g.Queue)+len(g.Balls))
	default:
		line2 = fmt.Sprintf("剩余 %d", len(g.Queue)+len(g.Balls))
	}
	col2 := rgba(0.62, 0.86, 1.0, 0.95)
	if g.State == game.StateLost && g.LoseReady() {
		col2 = rgba(1.0, 0.72, 0.55, 0.95)
	}
	r.drawText(vp, base, -0.74, 0.89, 38, line2, alignLeft, col2)

	// The win/lose overlay floats over the middle of the felt: with the table
	// tilting about its own centre, the near edge is the lowest part of the
	// playfield and text parked there ends up face-on to the table's front.
	overlay := hud.Mul4(mgl32.Translate3D(0, 0, -0.25))
	switch g.State {
	case game.StateWon:
		r.drawText(vp, overlay, 0, 1.06, 64, fmt.Sprintf("第 %d 关 通过!", g.Level), alignCenter, rgba(1.0, 0.93, 0.55, 1))
		r.drawText(vp, overlay, 0, 0.74, 44, fmt.Sprintf("得分 %d", g.Score), alignCenter, rgba(1, 1, 1, 0.96))
		hint := 0.72 + 0.28*float32(math.Sin(now*3.4))
		r.drawText(vp, overlay, 0, 0.46, 34, "按 扳机 进入下一关", alignCenter, rgba(1, 1, 1, hint))
	case game.StateLost:
		if g.LoseReady() {
			pulse := 0.80 + 0.20*float32(math.Sin(now*4))
			r.drawText(vp, overlay, 0, 1.06, 64, "游戏结束", alignCenter, rgba(1.0, 0.28, 0.20, pulse))
			r.drawText(vp, overlay, 0, 0.74, 44, fmt.Sprintf("得分 %d   最高 %d", g.Score, g.Best), alignCenter, rgba(1, 1, 1, 0.96))
		}
	}
}

// textAdvance reports a line's width in the art pixels the splash is drawn in.
func textAdvance(face []byte, text string, px int) float32 {
	l, err := font.Render(face, text, px)
	if err != nil {
		return 0
	}
	return float32(l.W)
}

// alignMode selects where (x,y) sits on a text line.
type alignMode int

const (
	alignLeft alignMode = iota
	alignCenter
	alignRight
)

// drawText draws one line whose top edge starts at y in the plane of base; x is
// the left edge, the centre or the right edge depending on align.
func (r *Renderer) drawText(vp, base mgl32.Mat4, x, y float32, px int, text string, align alignMode, col mgl32.Vec4) {
	r.drawTextFace(vp, base, x, y, px, text, align, col, font.Embedded(), "t")
}

// drawTextFace is drawText with an explicit face: the GitHub mark lives in its
// own glyph subset, so its texture cache entry needs a separate key prefix.
func (r *Renderer) drawTextFace(vp, base mgl32.Mat4, x, y float32, px int, text string, align alignMode, col mgl32.Vec4, face []byte, tag string) {
	if text == "" {
		return
	}
	key := fmt.Sprintf("%s|%s|%d", tag, text, px)
	line := r.texts[key]
	if line == nil {
		l, err := font.Render(face, text, px)
		if err != nil {
			return
		}
		line = r.uploadText(l.W, l.H, l.Mask)
		if line == nil {
			return
		}
		r.texts[key] = line
		// The score line keeps changing, so cap the atlas. Evict the least
		// interesting lines rather than leaking a texture per score.
		if len(r.texts) > 64 {
			for k, v := range r.texts {
				if k == key {
					continue
				}
				gl.DeleteTexture(v.tex)
				delete(r.texts, k)
				if len(r.texts) <= 32 {
					break
				}
			}
		}
	}
	const units = 0.0032
	w := float32(line.w) * units
	h := float32(line.h) * units
	switch align {
	case alignCenter:
		x -= w / 2
	case alignRight:
		x -= w
	}

	corner := func(dx, dy float32) [4]mgl32.Vec3 {
		var c [4]mgl32.Vec3
		local := [4][2]float32{{x + dx, y - h + dy}, {x + w + dx, y - h + dy}, {x + w + dx, y + dy}, {x + dx, y + dy}}
		for i, l := range local {
			p := base.Mul4x1(mgl32.Vec4{l[0], l[1], 0.004, 1})
			c[i] = mgl32.Vec3{p[0], p[1], p[2]}
		}
		return c
	}
	if debugText {
		c := corner(0, 0)
		n := vp.Mul4x1(mgl32.Vec4{c[3][0], c[3][1], c[3][2], 1})
		fmt.Printf("text %q px=%d size=%.3fx%.3f topLeftWorld=(%.3f,%.3f,%.3f) ndcY=%.3f\n", text, px, w, h, c[3][0], c[3][1], c[3][2], n[1]/n[3])
	}

	gl.Enable(gl.Blend)
	gl.BlendFunc(blendSrcAlpha, blendOneMinusSrcAlpha)
	gl.DepthMask(false)
	r.textProg.Bind()
	r.textProg.Mat4(r.uMVP, vp)
	r.textProg.Int(r.uTex, 0)
	gl.ActiveTexture(gl.Texture0)
	gl.BindTexture(gl.Texture2D, line.tex)

	r.quad.upload(corner(0.006, -0.006))
	r.textProg.Vec4(r.uColor, 0, 0, 0, col[3]*0.6)
	r.quad.draw()

	r.quad.upload(corner(0, 0))
	r.textProg.Vec4(r.uColor, col[0], col[1], col[2], col[3])
	r.quad.draw()

	gl.DepthMask(true)
	gl.Disable(gl.Blend)
}

func (r *Renderer) uploadText(w, h int, mask []byte) *textTex {
	if w <= 0 || h <= 0 || len(mask) < w*h {
		return nil
	}
	pix := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		pix[i*4+0] = 255
		pix[i*4+1] = 255
		pix[i*4+2] = 255
		pix[i*4+3] = mask[i]
	}
	tex := gl.GenTexture()
	gl.BindTexture(gl.Texture2D, tex)
	gl.PixelStore(gl.UnpackAlignment, 1)
	gl.TexImage2D(gl.Texture2D, int32(gl.RGBA8), int32(w), int32(h), gl.RGBA, gl.UnsignedByte, unsafe.Pointer(&pix[0]))
	gl.TexParamI(gl.Texture2D, gl.TextureMinFilter, int32(gl.Linear))
	gl.TexParamI(gl.Texture2D, gl.TextureMagFilter, int32(gl.Linear))
	gl.TexParamI(gl.Texture2D, gl.TextureWrapS, int32(gl.ClampToEdge))
	gl.TexParamI(gl.Texture2D, gl.TextureWrapT, int32(gl.ClampToEdge))
	return &textTex{tex: tex, w: w, h: h}
}

// ReadPixelsRGBA grabs the framebuffer into a PNG, flipped upright.
func ReadPixelsRGBA(path string, width, height int) error {
	buf := make([]byte, width*height*4)
	gl.PixelStore(gl.PackAlignment, 1)
	gl.ReadPixels(0, 0, int32(width), int32(height), buf)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		src := (height - 1 - y) * width * 4
		dst := y * width * 4
		copy(img.Pix[dst:dst+width*4], buf[src:src+width*4])
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
