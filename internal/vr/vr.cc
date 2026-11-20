// C++ translation unit: openvr.h is a C++ header, so the shim has to be C++
// with a plain C surface for cgo.
#include "vr.h"

#include <openvr.h>

#include <cstdint>
#include <cstdio>
#include <cstring>

namespace {

vr::IVRSystem *g_sys = nullptr;
vr::IVRCompositor *g_comp = nullptr;
vr::TrackedDevicePose_t g_poses[vr::k_unMaxTrackedDeviceCount];

// Action handles from the app manifest. Once an app loads a manifest SteamVR
// stops serving legacy controller state, so every button and axis govr needs
// has to come through these actions.
vr::VRActionSetHandle_t g_actionSet = vr::k_ulInvalidActionSetHandle;
vr::VRActionHandle_t g_actMove = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actTurn = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actTrigger = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actPause = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actModelNext = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actModelPrev = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actAnimNext = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actAnimPrev = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actScrubBack = vr::k_ulInvalidActionHandle;
vr::VRActionHandle_t g_actScrubFwd = vr::k_ulInvalidActionHandle;
vr::VRInputValueHandle_t g_leftHand = vr::k_ulInvalidInputValueHandle;
vr::VRInputValueHandle_t g_rightHand = vr::k_ulInvalidInputValueHandle;
bool g_actionsReady = false;

// readDigital asks for one action, optionally restricted to one hand. Checking
// the combined action and each hand separately matters because the combined
// query can report inactive while the hand that is actually pressing the
// button still has live data.
bool readDigital(vr::IVRInput *input, vr::VRActionHandle_t action,
                 vr::VRInputValueHandle_t source) {
    if (input == nullptr || action == vr::k_ulInvalidActionHandle) {
        return false;
    }
    vr::InputDigitalActionData_t data = {};
    if (input->GetDigitalActionData(action, &data, sizeof(data), source) !=
        vr::VRInputError_None) {
        return false;
    }
    return data.bActive && data.bState;
}

} // namespace

extern "C" {

int govr_vr_present(void) { return vr::VR_IsHmdPresent() ? 1 : 0; }

int govr_vr_init(char *err, int errlen) {
    if (err != nullptr && errlen > 0) {
        err[0] = '\0';
    }
    if (!vr::VR_IsHmdPresent()) {
        snprintf(err, static_cast<size_t>(errlen), "SteamVR reports no headset");
        return 1;
    }

    vr::EVRInitError e = vr::VRInitError_None;
    g_sys = vr::VR_Init(&e, vr::VRApplication_Scene);
    if (e != vr::VRInitError_None) {
        snprintf(err, static_cast<size_t>(errlen), "%s",
                 vr::VR_GetVRInitErrorAsEnglishDescription(e));
        g_sys = nullptr;
        return 2;
    }

    g_comp = vr::VRCompositor();
    if (g_comp == nullptr) {
        snprintf(err, static_cast<size_t>(errlen), "compositor unavailable");
        return 3;
    }

    g_comp->SetTrackingSpace(vr::TrackingUniverseStanding);
    memset(g_poses, 0, sizeof(g_poses));
    return 0;
}

void govr_vr_shutdown(void) {
    g_actionsReady = false;
    g_leftHand = vr::k_ulInvalidInputValueHandle;
    g_rightHand = vr::k_ulInvalidInputValueHandle;
    g_comp = nullptr;
    if (g_sys != nullptr) {
        vr::VR_Shutdown();
        g_sys = nullptr;
    }
}

int govr_vr_init_actions(const char *manifest_path, char *err, int errlen) {
    if (err != nullptr && errlen > 0) {
        err[0] = '\0';
    }
    if (g_sys == nullptr || manifest_path == nullptr) {
        return 0;
    }
    vr::IVRInput *input = vr::VRInput();
    if (input == nullptr) {
        snprintf(err, static_cast<size_t>(errlen), "VRInput unavailable");
        return 0;
    }
    vr::EVRInputError e = input->SetActionManifestPath(manifest_path);
    if (e != vr::VRInputError_None) {
        snprintf(err, static_cast<size_t>(errlen), "SetActionManifestPath returned %d", static_cast<int>(e));
        return 0;
    }
    struct {
        const char *path;
        vr::VRActionHandle_t *handle;
        const char *what;
    } wanted[] = {
        {"/actions/viewer/in/move", &g_actMove, "move"},
        {"/actions/viewer/in/turn", &g_actTurn, "turn"},
        {"/actions/viewer/in/trigger", &g_actTrigger, "trigger"},
        {"/actions/viewer/in/pause", &g_actPause, "pause"},
        {"/actions/viewer/in/model_next", &g_actModelNext, "model_next"},
        {"/actions/viewer/in/model_prev", &g_actModelPrev, "model_prev"},
        {"/actions/viewer/in/anim_next", &g_actAnimNext, "anim_next"},
        {"/actions/viewer/in/anim_prev", &g_actAnimPrev, "anim_prev"},
        {"/actions/viewer/in/scrub_back", &g_actScrubBack, "scrub_back"},
        {"/actions/viewer/in/scrub_fwd", &g_actScrubFwd, "scrub_fwd"},
        };
    if ((e = input->GetActionSetHandle("/actions/viewer", &g_actionSet)) != vr::VRInputError_None) {
        snprintf(err, static_cast<size_t>(errlen), "GetActionSetHandle returned %d", static_cast<int>(e));
        return 0;
    }
    for (const auto &w : wanted) {
        if ((e = input->GetActionHandle(w.path, w.handle)) != vr::VRInputError_None) {
            snprintf(err, static_cast<size_t>(errlen), "GetActionHandle(%s) returned %d", w.what,
                     static_cast<int>(e));
            return 0;
        }
    }
    g_leftHand = vr::k_ulInvalidInputValueHandle;
    g_rightHand = vr::k_ulInvalidInputValueHandle;
    input->GetInputSourceHandle("/user/hand/left", &g_leftHand);
    input->GetInputSourceHandle("/user/hand/right", &g_rightHand);
    g_actionsReady = true;
    return 1;
}

int govr_vr_actions(float *move_x, float *move_y, float *turn_x, float *turn_y, float *trigger,
                    int *pause, int *model_next, int *model_prev, int *anim_next, int *anim_prev,
                    int *scrub_back, int *scrub_fwd) {
    if (move_x != nullptr) *move_x = 0.0f;
    if (move_y != nullptr) *move_y = 0.0f;
    if (turn_x != nullptr) *turn_x = 0.0f;
    if (turn_y != nullptr) *turn_y = 0.0f;
    if (trigger != nullptr) *trigger = 0.0f;
    if (pause != nullptr) *pause = 0;
    if (model_next != nullptr) *model_next = 0;
    if (model_prev != nullptr) *model_prev = 0;
    if (anim_next != nullptr) *anim_next = 0;
    if (anim_prev != nullptr) *anim_prev = 0;
    if (scrub_back != nullptr) *scrub_back = 0;
    if (scrub_fwd != nullptr) *scrub_fwd = 0;
    if (!g_actionsReady || move_x == nullptr || move_y == nullptr || turn_x == nullptr ||
        turn_y == nullptr || trigger == nullptr || pause == nullptr || model_next == nullptr ||
        model_prev == nullptr || anim_next == nullptr || anim_prev == nullptr ||
        scrub_back == nullptr || scrub_fwd == nullptr) {
        return 0;
    }
    vr::IVRInput *input = vr::VRInput();
    if (input == nullptr) {
        return 0;
    }
    vr::VRActiveActionSet_t set = {};
    set.ulActionSet = g_actionSet;
    set.ulRestrictedToDevice = vr::k_ulInvalidInputValueHandle;
    if (input->UpdateActionState(&set, sizeof(set), 1) != vr::VRInputError_None) {
        // Do not drop to legacy input here: after a manifest load SteamVR
        // serves no legacy button state, so the old fallback would turn a
        // transient update error into a completely dead frame of input.
    }

    vr::InputAnalogActionData_t analog = {};
    const auto handle = vr::k_ulInvalidInputValueHandle;

    if (input->GetAnalogActionData(g_actMove, &analog, sizeof(analog), handle) == vr::VRInputError_None &&
        analog.bActive) {
        *move_x = analog.x;
        *move_y = analog.y;
    }
    if (input->GetAnalogActionData(g_actTurn, &analog, sizeof(analog), handle) == vr::VRInputError_None &&
        analog.bActive) {
        *turn_x = analog.x;
        *turn_y = analog.y;
    }
    if (input->GetAnalogActionData(g_actTrigger, &analog, sizeof(analog), handle) == vr::VRInputError_None &&
        analog.bActive) {
        *trigger = analog.x;
    }

    // The combined action can be inactive for a frame while one controller is
    // still reporting the button. Query both hands and OR the results.
    *pause = (readDigital(input, g_actPause, handle) ||
              readDigital(input, g_actPause, g_leftHand) ||
              readDigital(input, g_actPause, g_rightHand))
                 ? 1
                 : 0;
    *model_next = (readDigital(input, g_actModelNext, handle) ||
                   readDigital(input, g_actModelNext, g_leftHand) ||
                   readDigital(input, g_actModelNext, g_rightHand))
                      ? 1
                      : 0;
    *model_prev = (readDigital(input, g_actModelPrev, handle) ||
                   readDigital(input, g_actModelPrev, g_leftHand) ||
                   readDigital(input, g_actModelPrev, g_rightHand))
                      ? 1
                      : 0;
    *anim_next = (readDigital(input, g_actAnimNext, handle) ||
                  readDigital(input, g_actAnimNext, g_leftHand) ||
                  readDigital(input, g_actAnimNext, g_rightHand))
                     ? 1
                     : 0;
    *anim_prev = (readDigital(input, g_actAnimPrev, handle) ||
                  readDigital(input, g_actAnimPrev, g_leftHand) ||
                  readDigital(input, g_actAnimPrev, g_rightHand))
                     ? 1
                     : 0;
    *scrub_back = (readDigital(input, g_actScrubBack, handle) ||
                   readDigital(input, g_actScrubBack, g_leftHand) ||
                   readDigital(input, g_actScrubBack, g_rightHand))
                      ? 1
                      : 0;
    *scrub_fwd = (readDigital(input, g_actScrubFwd, handle) ||
                  readDigital(input, g_actScrubFwd, g_leftHand) ||
                  readDigital(input, g_actScrubFwd, g_rightHand))
                     ? 1
                     : 0;
    return 1;
}

void govr_vr_render_size(int *w, int *h) {
    uint32_t rw = 0;
    uint32_t rh = 0;
    if (g_sys != nullptr) {
        g_sys->GetRecommendedRenderTargetSize(&rw, &rh);
    }
    *w = static_cast<int>(rw);
    *h = static_cast<int>(rh);
}

void govr_vr_projection(int eye, float near_z, float far_z, float *out16) {
    memset(out16, 0, sizeof(float) * 16);
    if (g_sys == nullptr) {
        return;
    }
    const vr::HmdMatrix44_t m =
        g_sys->GetProjectionMatrix(static_cast<vr::EVREye>(eye), near_z, far_z);
    memcpy(out16, m.m, sizeof(m.m));
}

void govr_vr_eye_to_head(int eye, float *out12) {
    memset(out12, 0, sizeof(float) * 12);
    if (g_sys == nullptr) {
        return;
    }
    const vr::HmdMatrix34_t m = g_sys->GetEyeToHeadTransform(static_cast<vr::EVREye>(eye));
    memcpy(out12, m.m, sizeof(m.m));
}

int govr_vr_wait_poses(void) {
    if (g_comp == nullptr) {
        return -1;
    }
    vr::TrackedDevicePose_t poses[vr::k_unMaxTrackedDeviceCount];
    const vr::EVRCompositorError e =
        g_comp->WaitGetPoses(poses, vr::k_unMaxTrackedDeviceCount, nullptr, 0);
    if (e != vr::VRCompositorError_None) {
        return static_cast<int>(e);
    }
    memcpy(g_poses, poses, sizeof(poses));
    return 0;
}

int govr_vr_pose(int device, float *out12) {
    if (device < 0 || device >= static_cast<int>(vr::k_unMaxTrackedDeviceCount)) {
        return 0;
    }
    if (!g_poses[device].bPoseIsValid) {
        return 0;
    }
    memcpy(out12, g_poses[device].mDeviceToAbsoluteTracking.m, sizeof(float) * 12);
    return 1;
}

int govr_vr_submit(int eye, unsigned int texture) {
    if (g_comp == nullptr) {
        return -1;
    }
    vr::Texture_t tex;
    tex.handle = reinterpret_cast<void *>(static_cast<uintptr_t>(texture));
    tex.eType = vr::TextureType_OpenGL;
    tex.eColorSpace = vr::ColorSpace_Gamma;

    vr::VRTextureBounds_t bounds;
    bounds.uMin = 0.0f;
    bounds.vMin = 0.0f;
    bounds.uMax = 1.0f;
    bounds.vMax = 1.0f;

    const vr::EVRCompositorError e =
        g_comp->Submit(static_cast<vr::EVREye>(eye), &tex, &bounds, vr::Submit_Default);
    return static_cast<int>(e);
}

int govr_vr_poll_event(int *event_type) {
    if (g_sys == nullptr) {
        return 0;
    }
    vr::VREvent_t ev;
    memset(&ev, 0, sizeof(ev));
    if (!g_sys->PollNextEvent(&ev, sizeof(ev))) {
        return 0;
    }
    *event_type = static_cast<int>(ev.eventType);
    return 1;
}

int govr_vr_controller_aim(unsigned int device, float *out12) {
    if (g_sys == nullptr || device >= vr::k_unMaxTrackedDeviceCount) {
        return 0;
    }
    vr::VRControllerState_t state;
    memset(&state, 0, sizeof(state));
    vr::TrackedDevicePose_t pose;
    memset(&pose, 0, sizeof(pose));
    if (!g_sys->GetControllerStateWithPose(vr::TrackingUniverseStanding,
                                          static_cast<vr::TrackedDeviceIndex_t>(device),
                                          &state, sizeof(state), &pose)) {
        return 0;
    }
    if (!pose.bPoseIsValid) {
        return 0;
    }
    memcpy(out12, pose.mDeviceToAbsoluteTracking.m, sizeof(float) * 12);
    return 1;
}

int govr_vr_controller(int hand, unsigned int *device) {
    if (g_sys == nullptr) {
        return -1;
    }
    const vr::ETrackedControllerRole role =
        hand == 0 ? vr::TrackedControllerRole_LeftHand : vr::TrackedControllerRole_RightHand;
    const vr::TrackedDeviceIndex_t idx = g_sys->GetTrackedDeviceIndexForControllerRole(role);
    if (idx == vr::k_unTrackedDeviceIndexInvalid) {
        return -1;
    }
    *device = static_cast<unsigned int>(idx);
    return 0;
}

// govr_vr_controller_list enumerates controllers by device class, so a pad
// still works when SteamVR has not assigned it a left/right role.
int govr_vr_controller_list(unsigned int *out, int max) {
    if (g_sys == nullptr || out == nullptr) {
        return 0;
    }
    int n = 0;
    for (uint32_t i = 0; i < vr::k_unMaxTrackedDeviceCount && n < max; i++) {
        if (g_sys->GetTrackedDeviceClass(i) == vr::TrackedDeviceClass_Controller) {
            out[n++] = i;
        }
    }
    return n;
}

int govr_vr_controller_state(unsigned int device, float *axis_x, float *axis_y, float *trigger_axis,
                             unsigned long long *raw_buttons) {
    if (g_sys == nullptr || device >= vr::k_unMaxTrackedDeviceCount) {
        return 0;
    }
    vr::VRControllerState_t state;
    memset(&state, 0, sizeof(state));
    if (!g_sys->GetControllerState(static_cast<vr::TrackedDeviceIndex_t>(device), &state, sizeof(state))) {
        return 0;
    }

    *axis_x = state.rAxis[0].x;
    *axis_y = state.rAxis[0].y;
    // The trigger sits on rAxis[1] for wand-style controllers but on rAxis[2]
    // for the streamed Deckard pads used here, where rAxis[1] stays zero.
    *trigger_axis = state.rAxis[1].x > state.rAxis[2].x ? state.rAxis[1].x : state.rAxis[2].x;
    *raw_buttons = state.ulButtonPressed;
    return 1;
}

// Surface ulButtonTouched: some pads (the Deckard d-pad among them) only report
// a direction as touched, never as pressed, so the pressed mask alone cannot
// tell the buttons apart.
int govr_vr_controller_touched(unsigned int device, unsigned long long *raw_touched) {
    if (g_sys == nullptr || device >= vr::k_unMaxTrackedDeviceCount || raw_touched == nullptr) {
        return 0;
    }
    vr::VRControllerState_t state;
    memset(&state, 0, sizeof(state));
    if (!g_sys->GetControllerState(static_cast<vr::TrackedDeviceIndex_t>(device), &state, sizeof(state))) {
        return 0;
    }
    *raw_touched = state.ulButtonTouched;
    return 1;
}

int govr_vr_controller_axes(unsigned int device, float *out2, int count) {
    if (g_sys == nullptr || device >= vr::k_unMaxTrackedDeviceCount || out2 == nullptr) {
        return 0;
    }
    vr::VRControllerState_t state;
    memset(&state, 0, sizeof(state));
    if (!g_sys->GetControllerState(static_cast<vr::TrackedDeviceIndex_t>(device), &state, sizeof(state))) {
        return 0;
    }
    if (count > 5) {
        count = 5;
    }
    for (int i = 0; i < count; ++i) {
        out2[i * 2 + 0] = state.rAxis[i].x;
        out2[i * 2 + 1] = state.rAxis[i].y;
    }
    return 1;
}

} // extern "C"
