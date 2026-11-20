package render

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"

	"zuma/internal/game"
	"zuma/internal/gl"
	"zuma/internal/splashart"
)

// splashVS/FS draw the painted loading art as one textured quad. It samples the
// RGBA texture directly so the art keeps its own colours; the tint is used for
// the fade to black.
const splashVS = `#version 410 core
layout(location=0) in vec3 aPos;
layout(location=1) in vec2 aUV;
uniform mat4 uMVP;
out vec2 vUV;
void main() {
        vUV = aUV;
        gl_Position = uMVP * vec4(aPos, 1.0);
}`

const splashFS = `#version 410 core
in vec2 vUV;
uniform sampler2D uTex;
uniform vec4 uTint;
out vec4 frag;
void main() {
        frag = texture(uTex, vUV) * uTint;
}`

// splashTiming shapes the loading animation.
const (
	splashFadeIn  = 0.45
	splashFadeOut = 0.40
)

// splashState is the running loading animation.
type splashState struct {
	start float64
	dur   float64
	done  bool
}

// StartSplash arms the loading animation; dur <= 0 disables it.
func (r *Renderer) StartSplash(now, dur float64) {
	if dur <= 0 {
		r.splash = nil
		return
	}
	r.splash = &splashState{start: now, dur: dur}
}

// EndSplash drops the animation immediately (the player skipped it).
func (r *Renderer) EndSplash() { r.splash = nil }

// SplashActive reports whether the loading animation still owns the frame.
func (r *Renderer) SplashActive(now float64) bool {
	s := r.splash
	return s != nil && !s.done && now-s.start < s.dur
}

// splashProgress maps the elapsed time to the bar fraction. It eases out so the
// bar is quick off the mark and settles near the end.
func (s *splashState) progress(now float64) float32 {
	p := float32((now - s.start) / s.dur)
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	return 1 - (1-p)*(1-p)
}

// DrawSplash renders one loading frame. artWorld maps art pixels (x right, y
// up, origin bottom-left) into whatever space vp expects: a fullscreen ortho
// for the desktop window, or a head-locked panel inside the headset.
func (r *Renderer) DrawSplash(vp, artWorld mgl32.Mat4, now float64) {
	s := r.splash
	if s == nil || r.splashBG == 0 {
		return
	}
	mvp := vp.Mul4(artWorld)

	// Match the clear colour to the poster's dark edges so a letterboxed window
	// does not show a seam.
	gl.ClearColor(0.008, 0.014, 0.018, 1)
	gl.Clear(gl.ColorBufferBit | gl.DepthBufferBit)

	gl.Disable(gl.CullFace)
	gl.Disable(gl.DepthTest)

	// The painted cover.
	r.splashProg.Bind()
	r.splashProg.Mat4(r.uSplashMVP, mvp)
	r.splashProg.Int(r.uSplashTex, 0)
	gl.ActiveTexture(gl.Texture0)
	gl.BindTexture(gl.Texture2D, r.splashBG)
	r.splashProg.Vec4(r.uSplashTint, 1, 1, 1, 1)
	r.splashBGQuad.draw()

	// Beads run down the painted groove and vanish into the idol.
	r.meshProg.Bind()
	r.meshProg.Mat4(r.uVP, vp)
	r.meshProg.Vec3(r.uCam, 0, 0, 4000)
	gl.Enable(gl.DepthTest)
	gl.Enable(gl.CullFace)
	gl.CullFaceMode(gl.Back)
	r.meshProg.Float(r.uBall, 0)
	r.meshProg.Mat3(r.uBallRot, mgl32.Ident3())
	r.meshProg.Float(r.uGloss, 0)
	r.meshProg.Float(r.uUnlit, 0)
	spacing := float32(splashart.BeadSpacing)
	period := splashart.SpiralLength + spacing
	offset := float32(now-s.start) * 96
	count := int(period/spacing) + 1
	for i := 0; i < count; i++ {
		d := float32(i)*spacing + offset
		d = float32(math.Mod(float64(d), float64(period)))
		x, y, ok := splashart.SpiralAt(d)
		if !ok {
			continue
		}
		c := ballColor((i*3 + i/5) % len(game.Palette))
		// Grow in at the mouth of the groove and shrink into the drain, so
		// beads never pop in or out mid-air.
		u := d / splashart.SpiralLength
		radius := splashart.BeadRadius * ease(clamp01(u/0.018)) * ease(clamp01((1-u)/0.012))
		if radius < 0.5 {
			continue
		}
		model := artWorld.
			Mul4(mgl32.Translate3D(x, float32(splashart.ArtH)-y, 0)).
			Mul4(mgl32.Scale3D(radius, radius, radius))
		r.meshProg.Mat4(r.uModel, model)
		r.meshProg.Vec4(r.uTint, c[0], c[1], c[2], 1)
		r.meshProg.Float(r.uBall, 1)
		r.meshProg.Float(r.uGloss, 0)
		r.meshProg.Mat3(r.uBallRot, mat3Mul(rotAxis(mgl32.Vec3{0.6, 0.8, 0.2}, d/splashart.BeadRadius), randomBallRot(i*17+3)))
		r.ball.draw()
		r.meshProg.Float(r.uBall, 0)
	}

	// Dust motes drifting up through the light shafts.
	gl.Enable(gl.Blend)
	gl.BlendFunc(blendSrcAlpha, blendOneMinusSrcAlpha)
	r.meshProg.Float(r.uUnlit, 1)
	r.meshProg.Vec4(r.uTint, 1.0, 0.86, 0.58, 0.55)
	for i := 0; i < 26; i++ {
		h1 := float32(math.Mod(math.Sin(float64(i)*12.9898)*43758.5453, 1))
		h2 := float32(math.Mod(math.Sin(float64(i)*78.233)*12345.6789, 1))
		if h1 < 0 {
			h1 = -h1
		}
		if h2 < 0 {
			h2 = -h2
		}
		mx := 240 + 1450*h1
		my := 120 + 820*h2
		rise := float32(math.Mod((now-s.start)*26+float64(h2*900), 900))
		radius := 1.6 + 3.2*h1
		model := artWorld.
			Mul4(mgl32.Translate3D(mx, my+rise, 2)).
			Mul4(mgl32.Scale3D(radius, radius, radius))
		r.meshProg.Mat4(r.uModel, model)
		r.meshProg.Float(r.uBall, 0)
		r.ball.draw()
	}
	r.meshProg.Float(r.uUnlit, 0)
	gl.Disable(gl.Blend)

	// Progress fill over the painted track.
	p := s.progress(now)
	gl.Disable(gl.DepthTest)
	r.meshProg.Float(r.uUnlit, 1)
	r.meshProg.Vec4(r.uTint, 0.95, 0.75, 0.36, 1)
	if p > 0.001 {
		w := 632 * p
		model := artWorld.
			Mul4(mgl32.Translate3D(644, 125, 3)).
			Mul4(mgl32.Scale3D(w, 16, 1))
		r.meshProg.Mat4(r.uModel, model)
		r.splashBar.draw()
		// a brighter bead at the leading edge
		model = artWorld.
			Mul4(mgl32.Translate3D(644+w-8, 133, 4)).
			Mul4(mgl32.Scale3D(9, 9, 9))
		r.meshProg.Mat4(r.uModel, model)
		r.meshProg.Vec4(r.uTint, 1.0, 0.94, 0.74, 1)
		r.ball.draw()
		r.meshProg.Vec4(r.uTint, 0.95, 0.75, 0.36, 1)
	}
	r.meshProg.Float(r.uUnlit, 0)

	// Title, subtitle, progress label and the credit line.
	base := func(x, y float32) mgl32.Mat4 {
		return artWorld.Mul4(mgl32.Translate3D(x, y, 6)).Mul4(mgl32.Scale3D(312.5, 312.5, 312.5))
	}
	r.drawText(vp, base(146, 890), 0, 0, 168, "祖玛 VR", alignLeft, rgba(1.0, 0.86, 0.54, 1))
	r.drawText(vp, base(158, 668), 0, 0, 52, "球链神庙", alignLeft, rgba(0.55, 0.90, 0.84, 0.96))
	r.drawText(vp, base(960, 200), 0, 0, 34, fmt.Sprintf("加载中 %d%%", int(p*100)), alignCenter, rgba(0.86, 0.94, 0.92, 0.94))
	r.drawText(vp, base(1856, 64), 0, 0, 30, "github.com/lomoalbert", alignRight, rgba(0.66, 0.80, 0.84, 0.90))
	r.drawText(vp, base(150, 104), 0, 0, 26, "空格 / 扳机 跳过", alignLeft, rgba(0.52, 0.66, 0.68, 0.72))

	// Fade in from black and back out at the end.
	elapsed := now - s.start
	fade := float32(0)
	if elapsed < splashFadeIn {
		fade = 1 - float32(elapsed/splashFadeIn)
	}
	if left := s.dur - elapsed; left < splashFadeOut {
		if f := 1 - float32(left/splashFadeOut); f > fade {
			fade = f
		}
	}
	if fade > 0.001 {
		r.splashProg.Bind()
		r.splashProg.Mat4(r.uSplashMVP, mvp)
		r.splashProg.Vec4(r.uSplashTint, 0, 0, 0, fade)
		r.splashBGQuad.draw()
	}
	if elapsed >= s.dur {
		s.done = true
	}
}

// newSplashGeometry uploads the art texture and the two helper meshes.
func (r *Renderer) newSplashGeometry() error {
	img, err := png.Decode(bytes.NewReader(splashPNG))
	if err != nil {
		return fmt.Errorf("splash art: %w", err)
	}
	buf := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(buf, buf.Bounds(), img, img.Bounds().Min, draw.Src)

	tex := gl.GenTexture()
	gl.BindTexture(gl.Texture2D, tex)
	gl.PixelStore(gl.UnpackAlignment, 1)
	gl.TexImage2D(gl.Texture2D, int32(gl.RGBA8), int32(buf.Bounds().Dx()), int32(buf.Bounds().Dy()),
		gl.RGBA, gl.UnsignedByte, unsafe.Pointer(&buf.Pix[0]))
	gl.TexParamI(gl.Texture2D, gl.TextureMinFilter, int32(gl.Linear))
	gl.TexParamI(gl.Texture2D, gl.TextureMagFilter, int32(gl.Linear))
	gl.TexParamI(gl.Texture2D, gl.TextureWrapS, int32(gl.ClampToEdge))
	gl.TexParamI(gl.Texture2D, gl.TextureWrapT, int32(gl.ClampToEdge))
	r.splashBG = tex

	r.splashBGQuad = newTextQuad()
	w, h := float32(splashart.ArtW), float32(splashart.ArtH)
	r.splashBGQuad.upload([4]mgl32.Vec3{
		{0, 0, 0}, {w, 0, 0}, {w, h, 0}, {0, h, 0},
	})

	bar := &Builder{}
	bar.flatQuad(mgl32.Vec3{0, 0, 0}, mgl32.Vec3{1, 0, 0}, mgl32.Vec3{1, 1, 0}, mgl32.Vec3{0, 1, 0},
		rgba(1, 1, 1, 1))
	r.splashBar = upload(bar)

	sp, err := gl.NewProgram(splashVS, splashFS)
	if err != nil {
		return fmt.Errorf("splash shader: %w", err)
	}
	r.splashProg = sp
	r.uSplashMVP = sp.Loc("uMVP")
	r.uSplashTex = sp.Loc("uTex")
	r.uSplashTint = sp.Loc("uTint")
	return nil
}

// ease is a smoothstep used for the bead entrance and exit.
func ease(t float32) float32 { return t * t * (3 - 2*t) }

// clamp01 keeps a fraction inside [0,1].
func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// SplashFit maps the art rectangle onto a framebuffer, fitting the whole poster
// inside the frame at any window shape. Whatever margin is left over is filled
// by clamping the art's edge pixels (see DrawSplash), so the title and the
// credit line are never cropped.
func SplashFit(w, h int) mgl32.Mat4 {
	aw, ah := float32(splashart.ArtW), float32(splashart.ArtH)
	if w <= 0 || h <= 0 {
		w, h = 1, 1
	}
	c := float32(math.Min(float64(w)/float64(aw), float64(h)/float64(ah)))
	sx := 2 * c / float32(w)
	sy := 2 * c / float32(h)
	return mgl32.Mat4{
		sx, 0, 0, 0,
		0, sy, 0, 0,
		0, 0, 1e-3, 0,
		-c * aw / float32(w), -c * ah / float32(h), 0, 1,
	}
}

// SplashPanel places the art as a head-locked screen 2.6 m in front of the
// player, so the whole poster stays inside the headset's field of view.
func SplashPanel(head mgl32.Mat4) mgl32.Mat4 {
	const panelW = 4.6
	s := panelW / float32(splashart.ArtW)
	local := mgl32.Translate3D(0, 0, -2.6).
		Mul4(mgl32.Scale3D(s, s, s)).
		Mul4(mgl32.Translate3D(-splashart.ArtW/2, -splashart.ArtH/2, 0))
	return head.Mul4(local)
}
