// Package vr wraps the SteamVR/OpenVR runtime through a small C++ shim.
package vr

/*
#cgo CXXFLAGS: -I/usr/include/openvr
#cgo LDFLAGS: -lopenvr_api -lstdc++
#include <stdlib.h>
#include "vr.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
)

const (
	LeftHand  = 0
	RightHand = 1
)

// OpenVR event types that ask the application to exit (openvr.h VREvent_*).
const (
	vEventQuit                = 700
	vEventProcessQuit         = 701
	vEventDriverRequestedQuit = 704
)

// Button masks from openvr.h's ButtonMaskFromId(k_EButton_*): one bit per
// button id. The raw ulButtonPressed word is decoded here instead of in the
// C++ shim.
const (
	buttonMenu    uint64 = 1 << 1  // k_EButton_ApplicationMenu
	buttonGrip    uint64 = 1 << 2  // k_EButton_Grip
	buttonA       uint64 = 1 << 7  // k_EButton_A
	buttonStick   uint64 = 1 << 32 // k_EButton_SteamVR_Touchpad
	buttonTrigger uint64 = 1 << 33 // k_EButton_SteamVR_Trigger
)

// mat34 expands OpenVR's row-major 3x4 pose into the column-major 4x4 that
// mgl32 and OpenGL expect.
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

// transpose4 converts OpenVR's row-major 4x4 matrices to column-major.
func transpose4(raw [16]float32) mgl32.Mat4 {
	var m mgl32.Mat4
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			m[col*4+row] = raw[row*4+col]
		}
	}
	return m
}

// Present reports whether SteamVR sees a headset at all.
func Present() bool { return C.govr_vr_present() == 1 }

// Init brings up SteamVR in scene mode.
func Init() error {
	buf := C.malloc(512)
	defer C.free(buf)
	if code := C.govr_vr_init((*C.char)(buf), 512); code != 0 {
		return fmt.Errorf("%s", C.GoString((*C.char)(buf)))
	}
	return nil
}

func Shutdown() { C.govr_vr_shutdown() }

// InitActions loads the SteamVR action manifest that binds app actions such
// as the dash button, which the legacy button enum cannot express.
func InitActions(manifest string) error {
	path := C.CString(manifest)
	defer C.free(unsafe.Pointer(path))
	buf := C.malloc(256)
	defer C.free(buf)
	if code := C.govr_vr_init_actions(path, (*C.char)(buf), 256); code != 1 {
		return fmt.Errorf("%s", C.GoString((*C.char)(buf)))
	}
	return nil
}

// Actions is the viewer input read through the SteamVR action manifest: once
// a manifest is loaded SteamVR serves no legacy button state, so every control
// comes from here.
type Actions struct {
	MoveX, MoveY float32 // left stick: pan the model in the view plane
	TurnX, TurnY float32 // right stick: turn the camera, lift the model
	Trigger      float32 // right trigger pull: reset the view

	Pause, ModelNext, ModelPrev bool
	AnimNext, AnimPrev          bool
	ScrubBack, ScrubFwd         bool
}

// ReadActions samples the action set. ok is false when the manifest is not
// loaded (or the action set is not active), in which case callers fall back to
// Controller().
func ReadActions() (Actions, bool) {
	var moveX, moveY, turnX, turnY, trigger C.float
	var pause, modelNext, modelPrev, animNext, animPrev, scrubBack, scrubFwd C.int
	if C.govr_vr_actions(&moveX, &moveY, &turnX, &turnY, &trigger,
		&pause, &modelNext, &modelPrev, &animNext, &animPrev, &scrubBack, &scrubFwd) != 1 {
		return Actions{}, false
	}
	return Actions{
		MoveX:     float32(moveX),
		MoveY:     float32(moveY),
		TurnX:     float32(turnX),
		TurnY:     float32(turnY),
		Trigger:   float32(trigger),
		Pause:     pause != 0,
		ModelNext: modelNext != 0,
		ModelPrev: modelPrev != 0,
		AnimNext:  animNext != 0,
		AnimPrev:  animPrev != 0,
		ScrubBack: scrubBack != 0,
		ScrubFwd:  scrubFwd != 0,
	}, true
}

// RenderTargetSize returns SteamVR's recommended per-eye texture size.
func RenderTargetSize() (int32, int32) {
	var w, h C.int
	C.govr_vr_render_size(&w, &h)
	return int32(w), int32(h)
}

// Projection returns the eye projection matrix (OpenGL convention).
func Projection(eye int, near, far float32) mgl32.Mat4 {
	var raw [16]float32
	C.govr_vr_projection(C.int(eye), C.float(near), C.float(far), (*C.float)(unsafe.Pointer(&raw[0])))
	return transpose4(raw)
}

// EyeToHead is the fixed offset of the eye from the headset origin.
func EyeToHead(eye int) mgl32.Mat4 {
	var raw [12]float32
	C.govr_vr_eye_to_head(C.int(eye), (*C.float)(unsafe.Pointer(&raw[0])))
	return mat34(raw)
}

// WaitPoses blocks until the compositor has poses for this frame.
func WaitPoses() error {
	if code := C.govr_vr_wait_poses(); code != 0 {
		return fmt.Errorf("WaitGetPoses failed (code %d)", int(code))
	}
	return nil
}

// Pose returns the device-to-standing matrix for a tracked device.
func Pose(device int) (mgl32.Mat4, bool) {
	var raw [12]float32
	if C.govr_vr_pose(C.int(device), (*C.float)(unsafe.Pointer(&raw[0]))) != 1 {
		return mgl32.Ident4(), false
	}
	return mat34(raw), true
}

// Submit hands an eye texture to the compositor.
func Submit(eye int, texture uint32) error {
	if code := C.govr_vr_submit(C.int(eye), C.uint(texture)); code != 0 {
		return fmt.Errorf("Submit failed (code %d)", int(code))
	}
	return nil
}

// QuitRequested drains runtime events and reports whether SteamVR asked us to exit.
func QuitRequested() bool {
	for {
		var t C.int
		if C.govr_vr_poll_event(&t) == 0 {
			return false
		}
		switch int(t) {
		case vEventQuit, vEventProcessQuit, vEventDriverRequestedQuit:
			return true
		}
	}
}

// ControllerState is the subset of controller input govr uses.
type ControllerState struct {
	AxisX, AxisY     float32
	TriggerAxis      float32
	Raw              uint64
	Trigger, Grip, A bool
	Menu, Stick      bool
}

// ControllerAim returns the pose whose -Z axis points at the aim target.
func ControllerAim(device int) (mgl32.Mat4, bool) {
	var raw [12]float32
	if C.govr_vr_controller_aim(C.uint(device), (*C.float)(unsafe.Pointer(&raw[0]))) != 1 {
		return mgl32.Ident4(), false
	}
	return mat34(raw), true
}

// Touched is the ulButtonTouched mask for a device. Pads that only report a
// button as touched (rather than pressed) show up here and nowhere else.
func Touched(device uint32) (uint64, bool) {
	var raw C.ulonglong
	if C.govr_vr_controller_touched(C.uint(device), &raw) != 1 {
		return 0, false
	}
	return uint64(raw), true
}

// ControllerAxes returns the raw rAxis x/y pairs (up to 5) for a device.
func ControllerAxes(device uint32) ([5][2]float32, bool) {
	var out [10]float32
	if C.govr_vr_controller_axes(C.uint(device), (*C.float)(unsafe.Pointer(&out[0])), 5) != 1 {
		return [5][2]float32{}, false
	}
	var axes [5][2]float32
	for i := range axes {
		axes[i][0] = out[i*2]
		axes[i][1] = out[i*2+1]
	}
	return axes, true
}

// DeviceInfo is what SteamVR reports about a tracked device.
type DeviceInfo struct {
	Type           string
	TrackingSystem string
	Serial         string
	RenderModel    string
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// DeviceInfoOf reads the device description fields.
func DeviceInfoOf(dev int) DeviceInfo {
	typ := make([]byte, 128)
	trk := make([]byte, 128)
	ser := make([]byte, 128)
	mod := make([]byte, 256)
	C.govr_vr_device_info(C.uint(dev),
		(*C.char)(unsafe.Pointer(&typ[0])), C.int(len(typ)),
		(*C.char)(unsafe.Pointer(&trk[0])), C.int(len(trk)),
		(*C.char)(unsafe.Pointer(&ser[0])), C.int(len(ser)),
		(*C.char)(unsafe.Pointer(&mod[0])), C.int(len(mod)))
	return DeviceInfo{Type: cstr(typ), TrackingSystem: cstr(trk), Serial: cstr(ser), RenderModel: cstr(mod)}
}

// RenderModel is a controller mesh from SteamVR's IVRRenderModels.
type RenderModel struct {
	Name string
	Pos  []float32
	Nrm  []float32
	Idx  []uint16
}

// LoadRenderModel polls SteamVR's model loader: 1 = ready, 0 = still loading,
// -1 = failed. The mesh is in the controller's local space, already in metres.
func LoadRenderModel(dev int) (*RenderModel, int) {
	const maxVerts = 20000
	const maxIdx = 60000
	pos := make([]float32, maxVerts*3)
	nrm := make([]float32, maxVerts*3)
	idx := make([]uint16, maxIdx)
	var nv, ni C.int
	name := make([]byte, 256)
	state := int(C.govr_vr_model_load(C.uint(dev),
		(*C.float)(unsafe.Pointer(&pos[0])),
		(*C.float)(unsafe.Pointer(&nrm[0])),
		(*C.ushort)(unsafe.Pointer(&idx[0])),
		C.int(maxVerts), C.int(maxIdx), &nv, &ni,
		(*C.char)(unsafe.Pointer(&name[0])), C.int(len(name))))
	if state != 1 {
		return nil, state
	}
	end := 0
	for end < len(name) && name[end] != 0 {
		end++
	}
	return &RenderModel{
		Name: string(name[:end]),
		Pos:  pos[:int(nv)*3],
		Nrm:  nrm[:int(nv)*3],
		Idx:  idx[:int(ni)],
	}, 1
}

// ControllerDevices lists every tracked device SteamVR classifies as a
// controller, independent of the left/right role assignment.
func ControllerDevices() []int {
	out := make([]C.uint, 16)
	n := int(C.govr_vr_controller_list(&out[0], C.int(len(out))))
	if n <= 0 {
		return nil
	}
	devs := make([]int, 0, n)
	for i := 0; i < n; i++ {
		devs = append(devs, int(out[i]))
	}
	return devs
}

// ControllerAt reads the state of an explicit controller device.
func ControllerAt(dev uint32) (ControllerState, bool) {
	var ax, ay, trig C.float
	var raw C.ulonglong
	if C.govr_vr_controller_state(C.uint(dev), &ax, &ay, &trig, &raw) == 0 {
		return ControllerState{}, false
	}
	pressed := uint64(raw)
	return ControllerState{
		AxisX:       float32(ax),
		AxisY:       float32(ay),
		TriggerAxis: float32(trig),
		Raw:         pressed,
		Trigger:     pressed&buttonTrigger != 0,
		Grip:        pressed&buttonGrip != 0,
		A:           pressed&buttonA != 0,
		Menu:        pressed&buttonMenu != 0,
		Stick:       pressed&buttonStick != 0,
	}, true
}

// Controller resolves a hand role to a device and reads its current state.
func Controller(hand int) (uint32, ControllerState, bool) {
	var dev C.uint
	if C.govr_vr_controller(C.int(hand), &dev) != 0 {
		return 0, ControllerState{}, false
	}
	var ax, ay, trig C.float
	var raw C.ulonglong
	if C.govr_vr_controller_state(dev, &ax, &ay, &trig, &raw) == 0 {
		return 0, ControllerState{}, false
	}
	pressed := uint64(raw)
	return uint32(dev), ControllerState{
		AxisX:       float32(ax),
		AxisY:       float32(ay),
		TriggerAxis: float32(trig),
		Raw:         pressed,
		Trigger:     pressed&buttonTrigger != 0,
		Grip:        pressed&buttonGrip != 0,
		A:           pressed&buttonA != 0,
		Menu:        pressed&buttonMenu != 0,
		Stick:       pressed&buttonStick != 0,
	}, true
}
