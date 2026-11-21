package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/png"
	"log"
	"math"
	"os"
	"runtime"
	"time"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"

	"zuma/internal/audio"
	"zuma/internal/game"
	"zuma/internal/gl"
	"zuma/internal/gltf"
	"zuma/internal/render"
	"zuma/internal/vr"
	"zuma/internal/xr"
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
	vrOn           bool
	wantXR         bool
	xrOn           bool    // display goes through an OpenXR session
	xrPassthrough  bool    // clear the eye buffers transparent so the room shows
	xrTime         float64 // this frame's predicted display time
	eyes           [2]*gl.RenderTarget
	board          mgl32.Mat4
	boardInv       mgl32.Mat4
	placedBoard    game.Board
	tableY         float32 // table height: floor to the table origin, metres
	winAdvanceAt   float64 // win screen: when the next level loads
	tiltDeg        float32 // left stick: board tilt, degrees
	lastAimT       float64
	tableWorld     mgl32.Mat4
	tilt           mgl32.Mat4
	prevTrig       bool
	vrAimOK        bool
	vrAim          mgl32.Vec2
	vrDebug        bool
	vrcam          bool // documentation framing
	vrFrames       int
	lastVRLog      float64
	showOrigin     bool
	liftArmed      bool // right stick has been seen near neutral
	tiltArmed      bool // left stick has been seen near neutral
	vrRetryAt      float64
	vrFailLog      float64
	aimNote        string
	aimO           mgl32.Vec3
	aimD           mgl32.Vec3
	aimOK          bool
	aimFlip        bool
	aimFlipSet     bool
	flipSum        float32
	flipN          int
	aimSmooth      mgl32.Vec3
	aimSmoothOK    bool
	handPose       mgl32.Mat4
	handPoseOK     bool
	leftHandPose   mgl32.Mat4
	leftHandPoseOK bool
	// controller mesh: the aim ray leaves from the model, not the grip origin
	ctrlVerts       [2][]mgl32.Vec3
	ctrlOffset      [2]mgl32.Mat4
	ctrlOffsetOK    [2]bool
	ctrlTip         [2]mgl32.Vec3
	ctrlTipOK       [2]bool
	dpadPrev        bool // left d-pad edge detector for the passthrough toggle
	dpadAt          float64
	modelPoseLogged bool
	lastDev         int
	modelState      int
	ctrlModel       bool
	ctrlModelAt     float64 // next attempt for the XR render model fetch
	msaa            int32
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
	// The table has its own frame. Its origin is the centre of the felt; the
	// height below is the distance from that origin to the floor, the right
	// stick moves it, and the tilt turns the whole table about the same origin.
	tableHeight0 = 0.50
	tableMinY    = 0.15
	tableMaxY    = 1.65
	tableZ       = -1.55
	tableScale   = 0.55
	// The playfield leans toward the player: the near edge stays on the table
	// and the far edge rises, like a pinball table.
	tableTiltDeg = 20
	boardHingeZ  = 1.55
)

func main() {
	width := flag.Int("width", 1600, "window width")
	height := flag.Int("height", 900, "window height")
	desktop := flag.Bool("desktop", false, "force the desktop view even with a headset attached")
	useVR := flag.Bool("vr", true, "render in SteamVR when a headset is present")
	useXR := flag.Bool("xr", true, "use the OpenXR display path (needed for alpha-blend passthrough); falls back to OpenVR if it fails")
	passthrough := flag.Bool("passthrough", true, "with -xr: clear the background transparent instead of painting it")
	showOrigin := flag.Bool("origin", false, "draw the world origin axes (+X red, +Y green, +Z blue)")
	shot := flag.String("screenshot", "", "write a PNG before exiting (desktop view)")
	frames := flag.Int("frames", 40, "frames to render before the screenshot")
	seed := flag.Int64("seed", 0, "random seed (0 = time-based)")
	msaa := flag.Int("msaa", 4, "MSAA samples for the VR eye textures")
	vrDebug := flag.Bool("vr-debug", false, "log head/controller/aim once per second")
	ctrlModelHide := flag.Bool("controller-model-hide", false, "load and upload the controller mesh but do not draw it")
	ctrlModel := flag.Bool("controller-model", false, "load the controller mesh (XR: XR_EXT_render_model glTF; OpenVR: SteamVR render model)")
	aimX := flag.Float64("aim-x", 0, "board-space aim point x used by -screenshot")
	aimY := flag.Float64("aim-y", -0.5, "board-space aim point y used by -screenshot")
	fire := flag.Bool("fire", true, "fire one ball while rendering a screenshot")
	demo := flag.String("demo", "", "screenshot helper: \"won\" or \"lost\" shows the overlay")
	vrcam := flag.Bool("vrcam", false, "dev: first-person VR framing for documentation shots")
	title := flag.Bool("title", true, "show the title screen until the player presses start")
	yaw := flag.Float64("yaw", 0, "desktop camera yaw in radians")
	pitch := flag.Float64("pitch", 0.60, "desktop camera pitch in radians")
	dist := flag.Float64("dist", 6.2, "desktop camera distance in metres")
	flag.Bool("hud-debug", false, "print text quad sizes")
	flag.Parse()
	titleSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "title" {
			titleSet = true
		}
	})
	render.SetTextDebug(flag.Lookup("hud-debug").Value.String() == "true")

	// The OpenXR loader only lives inside the SteamVR install on the frame, so
	// point it at SteamVR's runtime manifest when nothing else did. A launcher
	// (Steam) does not pass this through.
	if os.Getenv("XR_RUNTIME_JSON") == "" {
		if _, err := os.Stat("/opt/steamvr/steamxr_linuxarm64.json"); err == nil {
			os.Setenv("XR_RUNTIME_JSON", "/opt/steamvr/steamxr_linuxarm64.json")
		}
	}

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

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	a := &app{
		g:   game.New(*seed),
		r:   renderer,
		cam: camera{yaw: *yaw, pitch: *pitch, dist: *dist},
	}
	if *shot != "" && !titleSet {
		*title = false // reference screenshots show the level, not the title art
	}
	splashStart := glfw.GetTime()
	if *shot != "" {
		splashStart = 0 // screenshots run on the fixed frame clock
	}
	renderer.StartSplash(splashStart, *title)

	a.tiltDeg = float32(tableTiltDeg)
	a.tableY = tableHeight0
	a.placeBoard()

	// Paint the loading cover before SteamVR comes up: vr.Init blocks for a
	// second or two, and an empty window looks like a crash.
	if *title {
		gl.BindFramebuffer(0)
		w, h := win.GetFramebufferSize()
		gl.Viewport(0, 0, int32(w), int32(h))
		clearScene()
		renderer.DrawSplash(mgl32.Ident4(), render.SplashFit(w, h), splashStart+1.0)
		win.SwapBuffers()
		log.Printf("title screen: press start to play")
	}

	a.vrDebug = *vrDebug
	a.vrcam = *vrcam
	if a.vrcam {
		a.cam.dist = 4.6 // tighter documentation framing
	}
	a.msaa = int32(*msaa)
	a.ctrlModel = *ctrlModel
	a.r.SetControllerHidden(*ctrlModelHide)
	a.wantXR = *useXR
	a.xrPassthrough = *passthrough
	a.showOrigin = *showOrigin
	// Screenshots are a desktop feature: the compositor owns the headset.
	canVR := *useVR && !*desktop && *shot == ""
	if canVR {
		if !a.enableVR() {
			log.Printf("headset not ready — desktop view, retrying every 2 s")
		}
	} else {
		log.Printf("desktop view")
	}
	if !a.vrOn && !a.xrOn {
		glfw.SwapInterval(1)
	}

	win.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
		if *shot != "" {
			// A screenshot is a fixed frame; a stray click must not restart it.
			return
		}
		if button == glfw.MouseButton1 && action == glfw.Press {
			if a.r.SplashActive(glfw.GetTime()) {
				a.r.SplashExit(glfw.GetTime())
				return
			}
			a.pressFire(glfw.GetTime())
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
			if a.r.SplashActive(glfw.GetTime()) {
				a.r.SplashExit(glfw.GetTime())
				return
			}
			a.pressFire(glfw.GetTime())
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
			// The screenshot clock counts frames, not wall time: the loop has
			// to outlive the window's opening animation, but the simulated
			// moment must stay the one -frames asks for.
			dt = 1.0 / 60.0
			f := frame
			if f > *frames {
				f = *frames
				dt = 0
			}
			now = float64(f) / 60.0
		} else {
			now = glfw.GetTime()
			dt = now - last
			last = now
			if dt > 0.1 {
				dt = 0.1
			}
		}

		xrReady := false
		if a.xrOn {
			if xr.Poll() {
				log.Printf("OpenXR requested shutdown")
				break
			}
			t, ok := xr.WaitFrame()
			if ok {
				xrReady = true
				a.xrTime = t
				if !xr.LocateViews(t) {
					log.Printf("xrLocateViews failed")
				}
				if !xr.SyncInput(t) {
					log.Printf("xrSyncActions failed")
				}
				if a.vrOn {
					if err := vr.WaitPoses(); err != nil {
						log.Printf("WaitGetPoses: %v", err)
					}
				}
				a.updateVRAim(now)
			}
		} else if a.vrOn {
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

		// -fire presses the trigger once, like a player would: a shot while
		// playing, the celebration burst on the win screen. A loss screen is
		// left alone, since its press would restart the round.
		if *shot != "" && *fire && frame == 8 &&
			(a.g.State == game.StatePlaying || a.g.State == game.StateWon) {
			a.pressFire(now)
		}
		if !a.r.SplashActive(now) {
			a.g.Update(float32(dt))
			a.r.SetPath(a.g.Board, a.g.Path)
			a.playEvents()
		}
		if a.g.Board != a.placedBoard {
			a.placeBoard()
		}
		if a.winAdvanceAt > 0 && now >= a.winAdvanceAt {
			a.winAdvanceAt = 0
			a.g.NextLevel()
			a.playEvents()
		}

		// XR path: the runtime owns the controller meshes, so they come from
		// XR_EXT_render_model as glTF instead of SteamVR's render model API.
		// Fetch and parse only for now; drawing comes next.
		if a.xrOn && a.ctrlModel && a.modelState < 2 && xrReady && now >= a.ctrlModelAt {
			// Retry quickly: the glTF fetch plus twelve 2k PNG decodes must
			// land while the title page is up, not in the middle of a level.
			a.ctrlModelAt = now + 0.5
			loaded := true
			for _, hand := range []int{xr.Left, xr.Right} {
				data, err := xr.ModelGLTF(hand)
				if err != nil {
					log.Printf("controller model hand %d: %v", hand, err)
					loaded = false
					continue
				}
				doc, err := gltf.ParseGLB(data)
				if err != nil {
					log.Printf("controller model hand %d: %v", hand, err)
					loaded = false
					continue
				}
				log.Printf("controller model hand %d: %d bytes (%s), %d nodes %v, %d verts, %d tris, %d materials, %d images",
					hand, len(data), doc.Asset.Generator, len(doc.Nodes), doc.NodeNames(),
					doc.Verts(), doc.Tris(), len(doc.Materials), len(doc.Images))
				// Upload every node of the controller (body, buttons, stick, trigger, ...)
				// as its own mesh. Textures are the next step; this pass is
				// flat-shaded.
				{
					a.r.ResetControllerModel(hand)
					a.ctrlVerts[hand] = a.ctrlVerts[hand][:0]
					a.ctrlTipOK[hand] = false
					for _, node := range doc.NodeNames() {
						p, n, uv, ii, img, nerr := doc.NodeMesh(node)
						if nerr != nil {
							log.Printf("controller model hand %d %s: %v", hand, node, nerr)
							continue
						}
						var tex uint32
						if img >= 0 {
							if raw, ierr := doc.ImageBytes(img); ierr == nil {
								if im, derr := png.Decode(bytes.NewReader(raw)); derr == nil {
									tex = a.r.NewTexture(im)
								} else {
									log.Printf("controller model hand %d %s: png: %v", hand, node, derr)
								}
							}
						}
						ix := make([]uint16, len(ii))
						bad := false
						for k, v := range ii {
							if v > 0xFFFF {
								bad = true
								break
							}
							ix[k] = uint16(v)
						}
						if bad {
							log.Printf("controller model hand %d %s: needs 32-bit indices", hand, node)
							continue
						}
						// status is a cover over the battery/hand-id display that the
						// body already paints; drawing it on top hides that artwork
						// behind its dark texels, so it is skipped.
						if node == "status" {
							continue
						}
						a.r.AddControllerNodeUV(hand, p, n, uv, ix, tex)
						if hand == xr.Right {
							for i := 0; i+2 < len(p); i += 3 {
								a.ctrlVerts[hand] = append(a.ctrlVerts[hand], mgl32.Vec3{p[i], p[i+1], p[i+2]})
							}
						}
						log.Printf("controller model hand %d %s: %d verts, %d tris", hand, node, len(p)/3, len(ix)/3)
					}
				}
			}
			if loaded {
				a.modelState = 2
			}
		}
		if a.vrOn && *ctrlModel && a.modelState == 0 && a.lastDev > 0 {
			m, state := vr.LoadRenderModel(a.lastDev)
			switch state {
			case 1:
				a.r.SetControllerModel(xr.Right, m.Pos, m.Nrm, m.Idx, m.Name)
				a.modelState = 2
				log.Printf("controller model: %s (%d verts)", m.Name, len(m.Pos)/3)
			case -1:
				a.modelState = -1
				log.Printf("controller model unavailable")
			}
		}
		if a.xrOn {
			if a.ctrlModel && a.modelState == 2 {
				for _, hand := range []int{xr.Left, xr.Right} {
					if mp, ok := xr.ModelPose(hand, a.xrTime); ok {
						a.r.SetControllerOffset(hand, mp)
						if !a.modelPoseLogged {
							log.Printf("controller model hand %d space: pos=(%.3f,%.3f,%.3f)",
								hand, mp[12], mp[13], mp[14])
						}
					}
				}
				a.modelPoseLogged = true
			}
			a.r.SetLeftHand(a.leftHandPose, a.leftHandPoseOK && a.ctrlModel && a.modelState == 2)
			if xrReady {
				a.renderEyesXR(a.xrTime, now)
			}
			a.renderMirror(now)
		} else if a.vrOn {
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
	if a.xrOn {
		xr.Shutdown()
	}
}

// enableVR brings SteamVR up and allocates the eye targets. It is retried from
// the main loop so the game can start before a wireless headset finishes
// connecting.
func (a *app) enableVR() bool {
	if a.wantXR && !a.xrOn {
		if err := a.startXR(); err != nil {
			log.Printf("OpenXR unavailable (%v) — falling back to OpenVR presentation", err)
		}
	}
	// With XR up, SteamVR must not be attached at all. An OpenVR client sitting
	// next to the XR session makes the runtime apply "reset centre" to the world
	// twice, so every recentre doubles the offset between the game world and the
	// room. Poses and buttons all come from the OpenXR actions instead.
	if !a.xrOn && !vr.Present() {
		return false
	}

	var err error
	if !a.xrOn {
		err = vr.Init()
	}
	if err != nil {
		if a.xrOn {
			xr.Shutdown()
			a.xrOn = false
		}
		now := glfw.GetTime()
		if now-a.vrFailLog > 10 {
			a.vrFailLog = now
			log.Printf("OpenVR init failed: %v", err)
		}
		return false
	}
	a.vrOn = !a.xrOn

	ew, eh := int32(0), int32(0)
	format := gl.RGBA8
	if a.xrOn {
		ew, eh = xr.Size()
		format = xr.Format()
	} else {
		ew, eh = vr.RenderTargetSize()
	}
	for eye := 0; eye < 2; eye++ {
		rt, err := gl.NewRenderTargetFormat(ew, eh, a.msaa, format)
		if err != nil {
			log.Fatalf("eye render target: %v", err)
		}
		a.eyes[eye] = rt
	}
	glfw.SwapInterval(0)
	if a.xrOn {
		log.Printf("OpenXR active: %dx%d per eye, msaa %dx, blend %d format 0x%x (passthrough %v, single client)",
			ew, eh, a.msaa, xr.BlendMode(), format, a.xrPassthrough)
		xr.SetPassthrough(a.xrPassthrough) // align the blend mode with the clear colour
	} else {
		log.Printf("VR active: %dx%d per eye, msaa %dx", ew, eh, a.msaa)
	}
	return true
}

// startXR opens the OpenXR session on the window's own GLX context.
func (a *app) startXR() error {
	dpy := glfw.GetX11Display()
	if dpy == nil {
		return fmt.Errorf("no X11 display")
	}
	xid := uintptr(currentWindow.GetX11Window())
	if err := xr.Init(unsafe.Pointer(dpy), xid); err != nil {
		return err
	}
	a.xrOn = true
	return nil
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
				// Each combo step gets a higher, louder impact so the level is
				// audible without watching the HUD.
				pitch := 1.0 + 0.09*float32(e.Value-1)
				gain := 0.45 + 0.06*float32(e.Value)
				if gain > 1.0 {
					gain = 1.0
				}
				audio.PlayPitched(audio.Combo, gain, pitch)
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
	// The desktop window renders the board in its own frame (no table offset),
	// tilted about its centre; the target sits below the origin so the felt
	// stays framed now that the tilt no longer lifts the board.
	target := mgl32.Vec3{0, 0.42, -0.15}
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
	if a.r.SplashActive(now) {
		a.r.DrawSplash(mgl32.Ident4(), render.SplashFit(w, h), now)
		return
	}
	a.r.DrawStatic(vp, mgl32.Ident4())
	a.r.Draw(vp, eye, a.tilt, mgl32.Ident4(), a.g, now, a.aimO, a.aimD, a.aimOK, a.handPose, a.handPoseOK)
	if a.showOrigin {
		a.drawOrigin(vp)
	}
}

func (a *app) renderMirror(now float64) {
	head, ok := a.headPose()
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
	if a.r.SplashActive(now) {
		a.r.DrawSplash(proj.Mul4(view), render.SplashPanel(), now)
		return
	}
	a.r.DrawStatic(proj.Mul4(view), a.board)
	a.r.Draw(proj.Mul4(view), cam, a.board, a.hudFrame(), a.g, now, a.aimO, a.aimD, a.aimOK, a.handPose, a.handPoseOK)
	if a.showOrigin {
		a.drawOrigin(proj.Mul4(view))
	}
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
		if a.r.SplashActive(now) {
			a.r.DrawSplash(proj.Mul4(view), render.SplashPanel(), now)
		} else {
			a.r.Draw(proj.Mul4(view), cam, a.board, a.hudFrame(), a.g, now, a.aimO, a.aimD, a.aimOK, a.handPose, a.handPoseOK)
			if a.showOrigin {
				a.drawOrigin(proj.Mul4(view))
			}
		}
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

// clearEye clears the eye buffer for the OpenXR path. With passthrough on it
// clears to transparent black so the runtime composites the room behind
// whatever the game then draws.
func (a *app) clearEye() {
	if a.xrPassthrough {
		gl.ClearColor(0, 0, 0, 0)
	} else {
		gl.ClearColor(0.012, 0.020, 0.032, 1)
	}
	gl.Clear(gl.ColorBufferBit | gl.DepthBufferBit)
}

// renderEyesXR draws both eyes into the runtime's swapchain images and submits
// the projection layer, which is the path that can carry alpha.
func (a *app) renderEyesXR(displayTime, now float64) {
	for eye := 0; eye < 2; eye++ {
		rt := a.eyes[eye]
		if rt == nil {
			continue
		}
		tex, ok := xr.Acquire(eye)
		if !ok {
			log.Printf("swapchain image unavailable for eye %d", eye)
			continue
		}
		rt.SetColorTexture(tex)
		proj := xr.Projection(eye, 0.05, 100)
		pose := xr.EyePose(eye)
		view := pose.Inv()
		cam := mgl32.Vec3{pose[12], pose[13], pose[14]}
		rt.Bind()
		gl.Viewport(0, 0, rt.W, rt.H)
		a.clearEye()
		if a.r.SplashActive(now) {
			a.r.DrawSplash(proj.Mul4(view), render.SplashPanel(), now)
		} else {
			a.r.Draw(proj.Mul4(view), cam, a.board, a.hudFrame(), a.g, now, a.aimO, a.aimD, a.aimOK, a.handPose, a.handPoseOK)
			if a.showOrigin {
				a.drawOrigin(proj.Mul4(view))
			}
		}
		rt.Resolve()
		gl.Flush()
		rt.Unbind()
		a.vrFrames++
	}
	if err := xr.EndFrame(displayTime); err != nil {
		log.Printf("xrEndFrame: %v", err)
	}
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
	inv := vp.Inv()
	na := inv.Mul4x1(mgl32.Vec4{ndcX, ndcY, -1, 1})
	nb := inv.Mul4x1(mgl32.Vec4{ndcX, ndcY, 1, 1})
	if na[3] == 0 || nb[3] == 0 {
		return
	}
	pa := mgl32.Vec3{na[0] / na[3], na[1] / na[3], na[2] / na[3]}
	pb := mgl32.Vec3{nb[0] / nb[3], nb[1] / nb[3], nb[2] / nb[3]}
	d := pb.Sub(pa)
	if d.Len() < 0.1 {
		return
	}
	d = d.Normalize()
	// The playfield is tilted, so work the ray in its own space.
	bo, bd := a.boardRay(pa, d)
	_ = eye
	a.aimO = bo.Add(bd.Mul(0.55))
	a.aimD = bd
	a.aimOK = true
	if a.vrcam {
		// Documentation shot: the right hand holds the launcher over the near
		// right of the table and its beam crosses the board.
		a.aimO = mgl32.Vec3{1.05, 1.32, 0.35}
		a.aimD = mgl32.Vec3{0.05, 1.05, -1.55}.Sub(a.aimO).Normalize()
	}
}

// drawOrigin is the -origin debug overlay: world axes plus a marker showing
// where the table centre should be if the scene follows the runtime exactly.
func (a *app) drawOrigin(vp mgl32.Mat4) {
	head, ok := a.mustHead()
	a.r.DrawAxes(vp, head, ok)
}

// mustHead is headPose with a usable fallback, for debug overlays.
func (a *app) mustHead() (mgl32.Mat4, bool) {
	if head, ok := a.headPose(); ok {
		return head, true
	}
	return mgl32.Ident4(), false
}

// headPose is the head matrix from whichever backend owns the display.
func (a *app) headPose() (mgl32.Mat4, bool) {
	if a.xrOn {
		return xr.HeadPose(a.xrTime)
	}
	return vr.Pose(0)
}

// rightPose is the right controller's grip pose.
func (a *app) rightPose(dev uint32) (mgl32.Mat4, bool) {
	if a.xrOn {
		h := xr.Hand(xr.Right)
		return h.Pose, h.PoseOK
	}
	return vr.Pose(int(dev))
}

// leftStick is the left controller's stick state.
func (a *app) leftStick() (vr.ControllerState, bool) {
	if a.xrOn {
		h := xr.Hand(xr.Left)
		if !h.PoseOK {
			return vr.ControllerState{}, false
		}
		return vr.ControllerState{AxisX: h.StickX, AxisY: h.StickY,
			Trigger: h.TriggerClick || h.Trigger > 0.5}, true
	}
	_, st, ok := vr.Controller(vr.LeftHand)
	return st, ok
}

// updateVRAim takes the right controller's aim pose as the shot ray. When no
// controller is tracked it falls back to the head gaze, so a laser is always
// visible in the headset.
func (a *app) updateVRAim(now float64) {
	var dev uint32
	var st vr.ControllerState
	var ok bool
	if !a.xrOn {
		dev, st, ok = vr.Controller(vr.RightHand)
		if !ok {
			for _, d := range vr.ControllerDevices() {
				if s, sok := vr.ControllerAt(uint32(d)); sok {
					dev, st, ok = uint32(d), s, true
					break
				}
			}
		}
	}
	if a.xrOn {
		// SteamVR only delivers buttons to the app that owns the session's
		// action bindings, so the background OpenVR client gets poses but
		// never a trigger. Input therefore comes from the OpenXR actions.
		right := xr.Hand(xr.Right)
		if right.PoseOK {
			st = vr.ControllerState{
				AxisX:       right.StickX,
				AxisY:       right.StickY,
				TriggerAxis: right.Trigger,
				Trigger:     right.TriggerClick || right.Trigger > 0.5,
				A:           right.Buttons&xr.ButtonA != 0,
				Stick:       right.Buttons&xr.ButtonStick != 0,
			}
			ok = true
		} else {
			ok = false
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
	// Right stick up/down raises or lowers the whole scene (table, board, HUD).
	{
		const liftSpeed = 0.6 // metres per second at full deflection
		dt := now - a.lastAimT
		if dt < 0 || dt > 0.25 {
			dt = 1.0 / 90.0
		}
		a.lastAimT = now
		// A stick that already reads deflection when the app starts (hand
		// resting on it) must not slam the table to its limit: each axis is
		// only trusted after it has been seen near neutral once.
		if !a.liftArmed {
			a.liftArmed = math.Abs(float64(st.AxisY)) < 0.2
		} else if v := st.AxisY; math.Abs(float64(v)) > 0.2 {
			a.tableY += v * liftSpeed * float32(dt)
			a.tableY = float32(math.Max(tableMinY, math.Min(tableMaxY, float64(a.tableY))))
			a.placeBoard()
		}
		// Left stick up/down trims the board tilt: up flattens, down steepens.
		if ls, lok := a.leftStick(); lok {
			if !a.tiltArmed {
				a.tiltArmed = math.Abs(float64(ls.AxisY)) < 0.2
			} else if v := ls.AxisY; math.Abs(float64(v)) > 0.2 {
				const tiltSpeed = 25.0 // degrees per second at full deflection
				a.tiltDeg -= v * tiltSpeed * float32(dt)
				a.tiltDeg = float32(math.Max(0, math.Min(35, float64(a.tiltDeg))))
				a.placeBoard()
			}
		}
	}

	pose, poseOK := a.rightPose(dev)
	if poseOK {
		th := mgl32.DegToRad(aimPitchDeg)
		local := mgl32.Vec3{0, -float32(math.Sin(float64(th))), -float32(math.Cos(float64(th)))}
		w := pose.Mul4x1(mgl32.Vec4{local[0], local[1], local[2], 0})
		d := mgl32.Vec3{w[0], w[1], w[2]}
		if d.Len() > 1e-4 {
			d = d.Normalize()
			o := mgl32.Vec3{pose[12], pose[13], pose[14]}
			// Fire from the controller mesh itself: the vertex furthest along
			// the aim direction, mapped through the runtime model space.
			if a.xrOn && a.ctrlModel && a.modelState == 2 {
				if mp, mok := xr.ModelPose(xr.Right, a.xrTime); mok {
					a.ctrlOffset[xr.Right], a.ctrlOffsetOK[xr.Right] = mp, true
					if !a.ctrlTipOK[xr.Right] && len(a.ctrlVerts[xr.Right]) > 0 {
						wd := mp.Inv().Mul4x1(mgl32.Vec4{local[0], local[1], local[2], 0})
						dm := mgl32.Vec3{wd[0], wd[1], wd[2]}
						if dm.Len() > 1e-4 {
							dm = dm.Normalize()
							best := float32(-1e9)
							var tip mgl32.Vec3
							for _, v := range a.ctrlVerts[xr.Right] {
								if k := v.Dot(dm); k > best {
									best, tip = k, v
								}
							}
							a.ctrlTip[xr.Right], a.ctrlTipOK[xr.Right] = tip, true
							log.Printf("controller ray tip: local=(%.3f,%.3f,%.3f) reach=%.3f", tip[0], tip[1], tip[2], best)
						}
					}
				}
			}
			if a.ctrlTipOK[xr.Right] {
				tip := a.ctrlTip[xr.Right]
				w := pose.Mul4(a.ctrlOffset[xr.Right]).Mul4x1(mgl32.Vec4{tip[0], tip[1], tip[2], 1})
				o = mgl32.Vec3{w[0], w[1], w[2]}
			}
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

	if a.xrOn {
		lh := xr.Hand(xr.Left)
		a.leftHandPose, a.leftHandPoseOK = lh.Pose, lh.PoseOK
	} else {
		a.leftHandPoseOK = false
	}

	// Left d-pad: any direction toggles passthrough. Only click edges count
	// (its touch surface covers all four directions at once), with a short
	// cooldown against bounce.
	if a.xrOn && !a.vrOn {
		down := xr.DpadAny()
		if down && !a.dpadPrev && now-a.dpadAt > 0.25 {
			a.dpadAt = now
			a.xrPassthrough = !a.xrPassthrough
			xr.SetPassthrough(a.xrPassthrough)
			log.Printf("passthrough -> %v", a.xrPassthrough)
		}
		a.dpadPrev = down
	}

	if st.Trigger && !a.prevTrig {
		if a.r.SplashActive(now) {
			// On the title page a trigger pull is "start", not a shot.
			a.r.SplashExit(now)
		} else {
			a.pressFire(now)
		}
	}
	a.prevTrig = st.Trigger
	a.logVR(now, st, ok, dev)
}

// placeBoard builds the table frame for the current level: the table's origin
// is the centre of the felt, at a.tableY above the floor, and the tilt turns
// the whole table (slab and playfield alike) about that origin. The table is
// one rigid body, so the felt can never sink into its own slab.
func (a *app) placeBoard() {
	bd := a.g.Board
	deg := a.tiltDeg
	tilt := mgl32.HomogRotate3DX(mgl32.DegToRad(deg))
	a.tilt = tilt
	a.tableWorld = mgl32.Translate3D(0, a.tableY, tableZ).
		Mul4(tilt).
		Mul4(mgl32.Scale3D(tableScale, tableScale, tableScale))
	// With the origin at the felt's centre the board and the table share one
	// frame: there is no hinge offset left to apply.
	a.board = a.tableWorld
	a.boardInv = a.board.Inv()
	if bd != a.placedBoard {
		half := bd.HalfH()
		near := a.board.Mul4x1(mgl32.Vec4{0, 0, half, 1}).Y()
		far := a.board.Mul4x1(mgl32.Vec4{0, 0, -half, 1}).Y()
		log.Printf("table %.2fx%.2f: height %.2f m, tilt %.1f deg, felt near %.2f / centre %.2f / far %.2f m, slab top %.2f m",
			bd.W, bd.H, a.tableY, deg, near, a.tableY, far, a.tableY-0.10*tableScale)
	}
	a.placedBoard = bd
}

// hudFrame is where the score and result text live: the table frame without its
// tilt, so the text stands upright in the room at every board angle.
func (a *app) hudFrame() mgl32.Mat4 {
	return mgl32.Translate3D(0, a.tableY, tableZ).
		Mul4(mgl32.Scale3D(tableScale, tableScale, tableScale))
}

// pressFire is the single fire button: on the win screen it throws the
// celebration burst and then loads the next level, after a loss it restarts,
// otherwise it fires a bead.
func (a *app) pressFire(now float64) {
	switch {
	case a.g.State == game.StateWon:
		if a.winAdvanceAt == 0 {
			a.g.DryVolley(a.aimO, a.aimD, 30)
			audio.Play(audio.Firework, 1.0)
			a.winAdvanceAt = now + 1.2
		}
	case a.g.LoseReady():
		a.g.Restart()
		a.playEvents()
	default:
		a.fire()
	}
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
	if head, ok := a.headPose(); ok {
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
	yaw := 0.0
	if head, ok := a.headPose(); ok {
		yaw = float64(mgl32.RadToDeg(float32(math.Atan2(float64(head[8]), float64(head[10])))))
	}
	eyaw := 0.0
	if a.xrOn {
		p := xr.EyePose(0)
		eyaw = float64(mgl32.RadToDeg(float32(math.Atan2(float64(p[8]), float64(p[10])))))
	}
	log.Printf("vr head=%s yaw=%.1f eyaw=%.1f dev=%d devO=%s left=%v right=%v aim=%s trig=%.2f btn=%v height=%.2f tilt=%.1f frames=%d", hp, yaw, eyaw, dev, devO, leftOK, rightOK, aim, st.TriggerAxis, st.Trigger, a.tableY, a.tiltDeg, a.vrFrames)
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
