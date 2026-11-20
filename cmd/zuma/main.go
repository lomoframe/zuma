package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"runtime"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"

	"zuma/internal/audio"
	"zuma/internal/game"
	"zuma/internal/gl"
	"zuma/internal/render"
	"zuma/internal/vr"
)

// camera is the desktop orbit around the board.
type camera struct {
	yaw, pitch, dist float64
}

type app struct {
	g   *game.Game
	r   *render.Renderer
	cam camera

	// VR state.
	vrOn        bool
	eyes        [2]*gl.RenderTarget
	board       mgl32.Mat4
	boardInv    mgl32.Mat4
	prevTrig    bool
	vrAimOK     bool
	vrAim       mgl32.Vec2
	vrDebug     bool
	vrFrames    int
	lastVRLog   float64
	vrRetryAt   float64
	vrFailLog   float64
	aimNote     string
	aimO        mgl32.Vec3
	aimD        mgl32.Vec3
	aimOK       bool
	aimFlip     bool
	aimFlipSet  bool
	flipSum     float32
	flipN       int
	aimSmooth   mgl32.Vec3
	aimSmoothOK bool
	handPose    mgl32.Mat4
	handPoseOK  bool
	lastDev     int
	modelState  int
	msaa        int32
}

// aimPitchDeg is how far the shot direction is pitched from the controller's
// -Z (blue) toward -Y (green), toward the physical tip.
const aimPitchDeg = 45

// pressForward pitches a direction down by deg around the world X axis. A
// world axis (rather than one derived from the direction) keeps the response
// stable even when the controller points nearly straight up or down.
func pressForward(d mgl32.Vec3, deg float32) mgl32.Vec3 {
	th := mgl32.DegToRad(-deg)
	c, sn := float32(math.Cos(float64(th))), float32(math.Sin(float64(th)))
	return mgl32.Vec3{
		d[0],
		d[1]*c - d[2]*sn,
		d[1]*sn + d[2]*c,
	}.Normalize()
}

const (
	vrBoardY     = 0.86
	vrBoardZ     = -1.55
	vrBoardScale = 0.55
)

func main() {
	width := flag.Int("width", 1600, "window width")
	height := flag.Int("height", 900, "window height")
	desktop := flag.Bool("desktop", false, "force the desktop view even with a headset attached")
	useVR := flag.Bool("vr", true, "render in SteamVR when a headset is present")
	shot := flag.String("screenshot", "", "write a PNG before exiting (desktop view)")
	frames := flag.Int("frames", 40, "frames to render before the screenshot")
	seed := flag.Int64("seed", 7, "random seed")
	msaa := flag.Int("msaa", 4, "MSAA samples for the VR eye textures")
	vrDebug := flag.Bool("vr-debug", false, "log head/controller/aim once per second")
	ctrlModel := flag.Bool("controller-model", false, "draw SteamVR's controller mesh (may crash the zink driver on the frame)")
	aimX := flag.Float64("aim-x", 0, "board-space aim point x used by -screenshot")
	aimY := flag.Float64("aim-y", -0.5, "board-space aim point y used by -screenshot")
	fire := flag.Bool("fire", true, "fire one ball while rendering a screenshot")
	demo := flag.String("demo", "", "screenshot helper: \"won\" or \"lost\" shows the overlay")
	yaw := flag.Float64("yaw", 0, "desktop camera yaw in radians")
	pitch := flag.Float64("pitch", 0.70, "desktop camera pitch in radians")
	dist := flag.Float64("dist", 4.6, "desktop camera distance in metres")
	flag.Bool("hud-debug", false, "print text quad sizes")
	flag.Parse()
	render.SetTextDebug(flag.Lookup("hud-debug").Value.String() == "true")

	runtime.LockOSThread()
	if err := glfw.Init(); err != nil {
		log.Fatalf("glfw: %v", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Samples, 0)
	win, err := glfw.CreateWindow(*width, *height, "祖玛 VR", nil, nil)
	if err != nil {
		log.Fatalf("window: %v", err)
	}
	win.MakeContextCurrent()
	currentWindow = win
	defer win.Destroy()

	if err := gl.Init(); err != nil {
		log.Fatalf("gl: %v", err)
	}
	fbW, fbH := win.GetFramebufferSize()
	log.Printf("GL: %s (window %dx%d, framebuffer %dx%d)", gl.Version(), *width, *height, fbW, fbH)

	renderer, err := render.New()
	if err != nil {
		log.Fatalf("renderer: %v", err)
	}
	if err := audio.Init(); err != nil {
		log.Printf("audio: %v (running silently)", err)
	} else {
		defer audio.Close()
	}

	a := &app{
		g:     game.New(*seed),
		r:     renderer,
		cam:   camera{yaw: *yaw, pitch: *pitch, dist: *dist},
		board: mgl32.Translate3D(0, vrBoardY, vrBoardZ).Mul4(mgl32.Scale3D(vrBoardScale, vrBoardScale, vrBoardScale)),
	}
	a.boardInv = a.board.Inv()

	a.vrDebug = *vrDebug
	a.msaa = int32(*msaa)
	// Screenshots are a desktop feature: the compositor owns the headset.
	canVR := *useVR && !*desktop && *shot == ""
	if canVR {
		if !a.enableVR() {
			log.Printf("headset not ready — desktop view, retrying every 2 s")
		}
	} else {
		log.Printf("desktop view")
	}
	if !a.vrOn {
		glfw.SwapInterval(1)
	}

	win.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
		if button == glfw.MouseButton1 && action == glfw.Press {
			a.fire()
		}
	})

	win.SetKeyCallback(func(w *glfw.Window, key glfw.Key, _ int, action glfw.Action, _ glfw.ModifierKey) {
		if action != glfw.Press && action != glfw.Repeat {
			return
		}
		switch key {
		case glfw.KeyEscape:
			w.SetShouldClose(true)
		case glfw.KeyR:
			a.g.Restart()
			a.playEvents()
			log.Printf("restarted")
		case glfw.KeySpace:
			a.fire()
		}
	})

	// The screenshot runs on a fixed clock so the same flags always produce the
	// same frame; interactive runs follow wall time.
	started := glfw.GetTime()
	last := started
	frame := 0
	shotDone := false
	if *shot != "" {
		target := mgl32.Vec3{float32(*aimX), 0.02, float32(*aimY)}
		hand := mgl32.Vec3{target[0] * 0.5, 0.95, target[2] + 1.6}
		a.aimO = hand
		a.aimD = target.Sub(hand).Normalize()
		a.aimOK = true
	}
	if *demo != "" {
		a.g.DemoState(*demo)
	}

	for {
		if win.ShouldClose() {
			break
		}
		var dt, now float64
		if *shot != "" {
			dt = 1.0 / 60.0
			now = float64(frame) / 60.0
		} else {
			now = glfw.GetTime()
			dt = now - last
			last = now
			if dt > 0.1 {
				dt = 0.1
			}
		}

		if a.vrOn {
			if vr.QuitRequested() {
				log.Printf("SteamVR requested shutdown")
				break
			}
			if err := vr.WaitPoses(); err != nil {
				log.Printf("WaitGetPoses: %v", err)
			}
			a.updateVRAim(now)
		} else if *shot == "" {
			a.updateMouseAim(win)
		}

		if *shot != "" && *fire && frame == 8 {
			a.fire()
		}
		a.g.Update(float32(dt))
		a.playEvents()

		if a.vrOn && *ctrlModel && a.modelState == 0 && a.lastDev > 0 {
			m, state := vr.LoadRenderModel(a.lastDev)
			switch state {
			case 1:
				a.r.SetControllerModel(m.Pos, m.Nrm, m.Idx, m.Name)
				a.modelState = 2
				log.Printf("controller model: %s (%d verts)", m.Name, len(m.Pos)/3)
			case -1:
				a.modelState = -1
				log.Printf("controller model unavailable")
			}
		}
		if a.vrOn {
			a.renderEyes(now)
			a.renderMirror(now)
		} else {
			a.renderDesktop(now)
		}

		if *shot != "" && !shotDone && frame+1 >= *frames && glfw.GetTime()-started >= 1.0 {
			w, h := win.GetFramebufferSize()
			if err := render.ReadPixelsRGBA(*shot, w, h); err != nil {
				log.Fatalf("screenshot: %v", err)
			}
			fmt.Println("wrote", *shot)
			shotDone = true
		}
		win.SwapBuffers()
		glfw.PollEvents()
		frame++
		if shotDone {
			break
		}
	}
	if a.vrOn {
		vr.Shutdown()
	}
}

// enableVR brings SteamVR up and allocates the eye targets. It is retried from
// the main loop so the game can start before a wireless headset finishes
// connecting.
func (a *app) enableVR() bool {
	if !vr.Present() {
		return false
	}
	if err := vr.Init(); err != nil {
		now := glfw.GetTime()
		if now-a.vrFailLog > 10 {
			a.vrFailLog = now
			log.Printf("OpenVR init failed: %v", err)
		}
		return false
	}
	a.vrOn = true
	ew, eh := vr.RenderTargetSize()
	for eye := 0; eye < 2; eye++ {
		rt, err := gl.NewRenderTarget(ew, eh, a.msaa)
		if err != nil {
			log.Fatalf("eye render target: %v", err)
		}
		a.eyes[eye] = rt
	}
	glfw.SwapInterval(0)
	log.Printf("VR active: %dx%d per eye, msaa %dx", ew, eh, a.msaa)
	return true
}

// playEvents maps gameplay moments to the synthesised clips.
func (a *app) playEvents() {
	for _, e := range a.g.TakeEvents() {
		switch e.Kind {
		case game.EventShoot:
			audio.Play(audio.Shoot, 0.55)
		case game.EventInsert:
			audio.Play(audio.Insert, 0.40)
		case game.EventMatch:
			audio.Play(audio.Pop, 0.8)
			if e.Value > 1 {
				audio.Play(audio.Combo, 0.55)
			}
		case game.EventWin:
			audio.Play(audio.Win, 0.8)
		case game.EventLose:
			audio.Play(audio.Lose, 0.85)
		case game.EventLevel:
			audio.Play(audio.Insert, 0.5)
		}
	}
}

func (a *app) viewProj(win *glfw.Window) (mgl32.Mat4, mgl32.Vec3) {
	w, h := win.GetFramebufferSize()
	return cameraVP(a.cam, w, h)
}

// cameraVP builds the desktop view-projection for a framebuffer size.
func cameraVP(cam camera, w, h int) (mgl32.Mat4, mgl32.Vec3) {
	if w == 0 || h == 0 {
		w, h = 1, 1
	}
	target := mgl32.Vec3{0, 0.26, -0.02}
	cp := math.Cos(cam.pitch)
	eye := mgl32.Vec3{
		target[0] + float32(cam.dist*cp*math.Sin(cam.yaw)),
		target[1] + float32(cam.dist*math.Sin(cam.pitch)),
		target[2] + float32(cam.dist*cp*math.Cos(cam.yaw)),
	}
	view := mgl32.LookAtV(eye, target, mgl32.Vec3{0, 1, 0})
	proj := mgl32.Perspective(mgl32.DegToRad(42), float32(w)/float32(h), 0.05, 80)
	return proj.Mul4(view), eye
}

func (a *app) renderDesktop(now float64) {
	vp, eye := a.viewProj(currentWindow)
	gl.BindFramebuffer(0)
	w, h := currentWindow.GetFramebufferSize()
	gl.Viewport(0, 0, int32(w), int32(h))
	clearScene()
	a.r.Draw(vp, eye, mgl32.Ident4(), a.g, now, a.aimO, a.aimD, a.aimOK, a.handPose, a.handPoseOK)
}

func (a *app) renderMirror(now float64) {
	head, ok := vr.Pose(0)
	if !ok {
		return
	}
	view := head.Mul4(vr.EyeToHead(0)).Inv()
	w, h := currentWindow.GetFramebufferSize()
	proj := mgl32.Perspective(mgl32.DegToRad(90), float32(w)/float32(h), 0.05, 100)
	eyeWorld := head.Mul4(vr.EyeToHead(0))
	cam := mgl32.Vec3{eyeWorld[12], eyeWorld[13], eyeWorld[14]}
	gl.BindFramebuffer(0)
	gl.Viewport(0, 0, int32(w), int32(h))
	clearScene()
	a.r.Draw(proj.Mul4(view), cam, a.board, a.g, now, a.aimO, a.aimD, a.aimOK, a.handPose, a.handPoseOK)
}

func (a *app) renderEyes(now float64) {
	head, ok := vr.Pose(0)
	if !ok {
		head = mgl32.Translate3D(0, 1.6, 0)
	}
	for eye := 0; eye < 2; eye++ {
		rt := a.eyes[eye]
		if rt == nil {
			continue
		}
		proj := vr.Projection(eye, 0.05, 100)
		view := head.Mul4(vr.EyeToHead(eye)).Inv()
		eyeWorld := head.Mul4(vr.EyeToHead(eye))
		cam := mgl32.Vec3{eyeWorld[12], eyeWorld[13], eyeWorld[14]}
		rt.Bind()
		gl.Viewport(0, 0, rt.W, rt.H)
		clearScene()
		a.r.Draw(proj.Mul4(view), cam, a.board, a.g, now, a.aimO, a.aimD, a.aimOK, a.handPose, a.handPoseOK)
		rt.Resolve()
		gl.Flush()
		rt.Unbind()
		if err := vr.Submit(eye, rt.Tex); err != nil {
			log.Printf("submit eye %d: %v", eye, err)
		}
		a.vrFrames++
	}
}

func clearScene() {
	gl.ClearColor(0.012, 0.020, 0.032, 1)
	gl.Clear(gl.ColorBufferBit | gl.DepthBufferBit)
}

// currentWindow lets the render helpers reach the GLFW window without threading
// it through every call.
var currentWindow *glfw.Window

// fire launches the loaded bead along the current aim ray.
func (a *app) fire() bool {
	if !a.aimOK {
		return false
	}
	ok := a.g.Fire(a.aimO, a.aimD)
	if ok && a.vrDebug {
		log.Printf("shot o=(%.2f,%.2f,%.2f) d=(%.2f,%.2f,%.2f)", a.aimO[0], a.aimO[1], a.aimO[2], a.aimD[0], a.aimD[1], a.aimD[2])
	}
	a.playEvents()
	return ok
}

// updateMouseAim turns the cursor into a board-space shot ray.
func (a *app) updateMouseAim(win *glfw.Window) {
	vp, eye := a.viewProj(win)
	ww, wh := win.GetSize()
	fw, fh := win.GetFramebufferSize()
	if ww <= 0 || wh <= 0 || fw <= 0 || fh <= 0 {
		return
	}
	cx, cy := win.GetCursorPos()
	px := cx * float64(fw) / float64(ww)
	py := cy * float64(fh) / float64(wh)
	ndcX := float32(2*px/float64(fw) - 1)
	ndcY := float32(1 - 2*py/float64(fh))
	p, ok := rayPlane(vp.Inv(), ndcX, ndcY, 0.02)
	if !ok {
		return
	}
	d := p.Sub(eye)
	if d.Len() < 0.1 {
		return
	}
	d = d.Normalize()
	a.aimO = eye.Add(d.Mul(0.55))
	a.aimD = d
	a.aimOK = true
}

// updateVRAim takes the right controller's aim pose as the shot ray. When no
// controller is tracked it falls back to the head gaze, so a laser is always
// visible in the headset.
func (a *app) updateVRAim(now float64) {
	dev, st, ok := vr.Controller(vr.RightHand)
	if !ok {
		for _, d := range vr.ControllerDevices() {
			if s, sok := vr.ControllerAt(uint32(d)); sok {
				dev, st, ok = uint32(d), s, true
				break
			}
		}
	}
	if !ok {
		a.aimOK = false
		a.aimNote = "no-controller"
		a.logVR(now, st, ok, dev)
		return
	}
	if a.lastDev != int(dev) {
		a.lastDev = int(dev)
		info := vr.DeviceInfoOf(int(dev))
		log.Printf("controller %d: type=%q tracking=%q serial=%q model=%q",
			dev, info.Type, info.TrackingSystem, info.Serial, info.RenderModel)
	}

	// Aim from the grip pose with the calibrated local direction: pitched from
	// -Z (blue) toward -Y (green) by aimPitchDeg, which is where the physical
	// tip is on the Quest 2 Touch controllers this project runs on.
	pose, poseOK := vr.Pose(int(dev))
	if poseOK {
		th := mgl32.DegToRad(aimPitchDeg)
		local := mgl32.Vec3{0, -float32(math.Sin(float64(th))), -float32(math.Cos(float64(th)))}
		w := pose.Mul4x1(mgl32.Vec4{local[0], local[1], local[2], 0})
		d := mgl32.Vec3{w[0], w[1], w[2]}
		if d.Len() > 1e-4 {
			d = d.Normalize()
			o := mgl32.Vec3{pose[12], pose[13], pose[14]}.Add(d.Mul(0.07))
			a.lastDev = int(dev)
			a.handPose = pose
			a.handPoseOK = true
			a.aimO, a.aimD = a.boardRay(o, d)
			if a.aimSmoothOK {
				a.aimD = a.aimSmooth.Mul(0.6).Add(a.aimD.Mul(0.4)).Normalize()
			}
			a.aimSmooth = a.aimD
			a.aimSmoothOK = true
			a.aimOK = true
		}
	} else {
		a.handPoseOK = false
		a.aimNote = "no-device-pose"
	}

	if st.Trigger && !a.prevTrig {
		if a.g.LoseReady() {
			a.g.Restart()
			a.playEvents()
		} else {
			a.fire()
		}
	}
	a.prevTrig = st.Trigger
	a.logVR(now, st, ok, dev)
}

// boardRay converts a world-space ray into board space.
func (a *app) boardRay(o, d mgl32.Vec3) (mgl32.Vec3, mgl32.Vec3) {
	po := a.boardInv.Mul4x1(mgl32.Vec4{o[0], o[1], o[2], 1})
	pd := a.boardInv.Mul4x1(mgl32.Vec4{o[0] + d[0], o[1] + d[1], o[2] + d[2], 1})
	bo := mgl32.Vec3{po[0], po[1], po[2]}
	bd := mgl32.Vec3{pd[0] - po[0], pd[1] - po[1], pd[2] - po[2]}
	if bd.Len() < 1e-6 {
		return bo, mgl32.Vec3{0, -1, 0}
	}
	return bo, bd.Normalize()
}

// logVR prints one diagnostic line per second in the headset.
func (a *app) logVR(now float64, st vr.ControllerState, rightOK bool, dev uint32) {
	if !a.vrDebug || now-a.lastVRLog < 1.0 {
		return
	}
	a.lastVRLog = now
	hp := "none"
	if head, ok := vr.Pose(0); ok {
		hp = fmt.Sprintf("(%.2f,%.2f,%.2f)", head[12], head[13], head[14])
	}
	_, _, leftOK := vr.Controller(vr.LeftHand)
	aim := "miss:" + a.aimNote
	if a.aimOK {
		aim = fmt.Sprintf("o=(%.2f,%.2f,%.2f) d=(%.2f,%.2f,%.2f)", a.aimO[0], a.aimO[1], a.aimO[2], a.aimD[0], a.aimD[1], a.aimD[2])
	}
	devO := "none"
	if a.handPoseOK {
		devO = fmt.Sprintf("(%.2f,%.2f,%.2f)", a.handPose[12], a.handPose[13], a.handPose[14])
	}
	log.Printf("vr head=%s dev=%d devO=%s left=%v right=%v aim=%s trig=%.2f btn=%v frames=%d", hp, dev, devO, leftOK, rightOK, aim, st.TriggerAxis, st.Trigger, a.vrFrames)
	a.vrFrames = 0
}

// rayPlane intersects an NDC ray with the y=plane horizontal plane.
func rayPlane(inv mgl32.Mat4, ndcX, ndcY, planeY float32) (mgl32.Vec3, bool) {
	na := inv.Mul4x1(mgl32.Vec4{ndcX, ndcY, -1, 1})
	nb := inv.Mul4x1(mgl32.Vec4{ndcX, ndcY, 1, 1})
	if na[3] == 0 || nb[3] == 0 {
		return mgl32.Vec3{}, false
	}
	a := mgl32.Vec3{na[0] / na[3], na[1] / na[3], na[2] / na[3]}
	b := mgl32.Vec3{nb[0] / nb[3], nb[1] / nb[3], nb[2] / nb[3]}
	d := b.Sub(a)
	if math.Abs(float64(d[1])) < 1e-6 {
		return mgl32.Vec3{}, false
	}
	t := (planeY - a[1]) / d[1]
	if t <= 0 {
		return mgl32.Vec3{}, false
	}
	return a.Add(d.Mul(t)), true
}
