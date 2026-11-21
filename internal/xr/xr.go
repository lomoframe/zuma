// Package xr binds the OpenXR display path: a GLX session whose projection
// layer is submitted with alpha blending, so pixels the game leaves
// transparent show the headset cameras.
package xr

/*
#cgo CXXFLAGS: -std=c++17
#cgo LDFLAGS: -lopenxr_loader -lGL -lX11
#include <stdlib.h>
#include "xr.h"
*/
import "C"

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
)

// Blend modes as OpenXR numbers them.
const (
	BlendOpaque     = 1
	BlendAlphaBlend = 3
)

// mat34 expands a row-major 3x4 pose into the column-major 4x4 mgl32 expects.
func mat34(raw [12]float32) mgl32.Mat4 {
	var m mgl32.Mat4
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			m[col*4+row] = raw[row*4+col]
		}
	}
	m[12], m[13], m[14], m[15] = raw[3], raw[7], raw[11], 1
	return m
}

// Init creates the instance and a session bound to the current GLX context.
// dpy is the X11 Display* of the window and xid its X window id.
func Init(dpy unsafe.Pointer, xid uintptr) error {
	buf := C.malloc(512)
	defer C.free(buf)
	if code := C.xr_init(dpy, C.ulong(xid), (*C.char)(buf), 512); code != 1 {
		return fmt.Errorf("%s", C.GoString((*C.char)(buf)))
	}
	return nil
}

func Shutdown() { C.xr_shutdown() }

// BlendMode reports the mode the runtime accepted for this session.
func BlendMode() int { return int(C.xr_blend_mode()) }

// Format is the GL internal format of the eye swapchains.
func Format() uint32 { return uint32(C.xr_swapchain_format()) }

// State is the current XrSessionState value.
func State() int { return int(C.xr_state()) }

// Size is the per-eye swapchain size.
func Size() (int32, int32) {
	var w, h C.int
	C.xr_size(&w, &h)
	return int32(w), int32(h)
}

// Poll drains runtime events and reports whether the app must exit.
func Poll() bool { return C.xr_poll() == 1 }

// WaitFrame blocks for the next frame. ok is false when the frame was already
// ended without layers because the session is not visible yet.
func WaitFrame() (time float64, ok bool) {
	var t C.double
	if C.xr_wait_frame(&t) != 1 {
		return 0, false
	}
	return float64(t), true
}

// LocateViews samples both eye views for the given display time.
func LocateViews(t float64) bool { return C.xr_locate_views(C.double(t)) == 1 }

// EyePose is the eye-to-local matrix of one view. Call it after LocateViews.
func EyePose(eye int) mgl32.Mat4 {
	var raw [12]float32
	C.xr_eye_pose(C.int(eye), (*C.float)(unsafe.Pointer(&raw[0])))
	return mat34(raw)
}

// EyeFov is the view frustum in radians: left, right, up, down.
func EyeFov(eye int) [4]float32 {
	var raw [4]float32
	C.xr_eye_fov(C.int(eye), (*C.float)(unsafe.Pointer(&raw[0])))
	return raw
}

// Projection builds the eye projection matrix from the located view frustum.
func Projection(eye int, near, far float32) mgl32.Mat4 {
	fov := EyeFov(eye)
	l := float32(math.Tan(float64(fov[0])))
	r := float32(math.Tan(float64(fov[1])))
	u := float32(math.Tan(float64(fov[2])))
	d := float32(math.Tan(float64(fov[3])))
	var m mgl32.Mat4
	m[0] = 2 / (r - l)
	m[5] = 2 / (u - d)
	m[8] = (r + l) / (r - l)
	m[9] = (u + d) / (u - d)
	m[10] = -(far + near) / (far - near)
	m[11] = -1
	m[14] = -2 * far * near / (far - near)
	return m
}

// HeadPose locates the VIEW space in the local space.
func HeadPose(t float64) (mgl32.Mat4, bool) {
	var raw [12]float32
	if C.xr_head_pose(C.double(t), (*C.float)(unsafe.Pointer(&raw[0]))) != 1 {
		return mgl32.Ident4(), false
	}
	return mat34(raw), true
}

// Acquire waits for the swapchain image of one eye and returns its GL texture.
func Acquire(eye int) (uint32, bool) {
	var tex C.uint
	if C.xr_acquire(C.int(eye), &tex) != 1 {
		return 0, false
	}
	return uint32(tex), true
}

// ModelGLTF fetches the runtime's controller mesh for one hand (0 = left, 1 =
// right) as glTF bytes. It needs the session running and the pose actions
// attached: the runtime only exposes models for profiles the app has bound.
func ModelGLTF(hand int) ([]byte, error) {
	buf := C.malloc(512)
	defer C.free(buf)
	var n C.int
	p := C.xr_model_gltf(C.int(hand), &n, (*C.char)(buf), 512)
	if p == nil {
		return nil, fmt.Errorf("%s", C.GoString((*C.char)(buf)))
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoBytes(unsafe.Pointer(p), n), nil
}

// ModelPose is the render model space of one hand inside that hand's grip
// space. Multiply it by the grip pose to place the mesh the runtime gave us.
func ModelPose(hand int, t float64) (mgl32.Mat4, bool) {
	var raw [12]float32
	if C.xr_model_pose(C.int(hand), C.double(t), (*C.float)(unsafe.Pointer(&raw[0]))) != 1 {
		return mgl32.Ident4(), false
	}
	return mat34(raw), true
}

// DpadAny reports whether any left-hand d-pad direction is held.
func DpadAny() bool { return C.xr_dpad_any() != 0 }

// SetPassthrough switches the environment blend mode at runtime: on shows the
// headset cameras wherever the app leaves pixels transparent.
func SetPassthrough(on bool) {
	v := C.int(0)
	if on {
		v = 1
	}
	C.xr_set_passthrough(v)
}

// Hands, as the runtime names the subaction paths.
const (
	Left  = 0
	Right = 1
)

// Button bits reported by Hand.
const (
	ButtonA     = 1
	ButtonB     = 2
	ButtonX     = 4
	ButtonY     = 8
	ButtonStick = 16
)

// HandState is one controller for the current frame.
type HandState struct {
	Pose         mgl32.Mat4
	PoseOK       bool
	Trigger      float32
	TriggerClick bool
	StickX       float32
	StickY       float32
	Buttons      int
}

// SyncInput samples the action set; call it once per frame after WaitFrame.
func SyncInput(t float64) bool { return C.xr_sync_input(C.double(t)) == 1 }

// Hand reads the last synced state of one controller.
func Hand(hand int) HandState {
	var raw [12]float32
	st := HandState{}
	if C.xr_hand_pose(C.int(hand), 0, (*C.float)(unsafe.Pointer(&raw[0]))) == 1 {
		st.Pose = mat34(raw)
		st.PoseOK = true
	}
	st.Trigger = float32(C.xr_hand_trigger(C.int(hand)))
	st.TriggerClick = C.xr_hand_trigger_click(C.int(hand)) == 1
	var x, y C.float
	C.xr_hand_stick(C.int(hand), &x, &y)
	st.StickX = float32(x)
	st.StickY = float32(y)
	st.Buttons = int(C.xr_hand_buttons(C.int(hand)))
	return st
}

// Haptic pulses one controller: amplitude 0..1, frequency in Hz (0 = runtime
// default), seconds = how long the pulse lasts.
func Haptic(hand int, amplitude, frequency, seconds float32) {
	C.xr_haptic(C.int(hand), C.float(amplitude), C.float(frequency), C.double(seconds))
}

// EndFrame releases the acquired images and submits the projection layer.
func EndFrame(t float64) error {
	if C.xr_end_frame(C.double(t)) != 1 {
		return fmt.Errorf("xrEndFrame failed")
	}
	return nil
}
