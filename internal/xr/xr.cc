// OpenXR display layer for the game. It owns a GLX-bound session, one GL
// swapchain per eye and submits them as a projection layer, asking the runtime
// for XR_ENVIRONMENT_BLEND_MODE_ALPHA_BLEND so cleared-out pixels show the
// headset cameras instead of black.
#define XR_USE_PLATFORM_XLIB
#define XR_USE_GRAPHICS_API_OPENGL

#include <X11/Xlib.h>
#include <GL/glx.h>
#include <openxr/openxr.h>
#include <openxr/openxr_platform.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <vector>

#include "xr.h"

namespace {

const char *kGLXExtension = "XR_KHR_opengl_enable";
// Steam Frame Controller's own interaction profile. The headers this project
// builds against predate it, so the name stays a literal; the runtime reports it
// through xrEnumerateInstanceExtensionProperties.
const char *kFrameControllerExtension = "XR_VALVE_frame_controller_interaction";
const char *kFrameControllerProfile = "/interaction_profiles/valve/frame_controller_valve";
const int kMaxSwapchainImages = 8;

XrInstance g_instance = XR_NULL_HANDLE;
XrSystemId g_system = XR_NULL_SYSTEM_ID;
XrSession g_session = XR_NULL_HANDLE;
XrSpace g_local = XR_NULL_HANDLE;
XrSpace g_viewSpace = XR_NULL_HANDLE;
XrEnvironmentBlendMode g_blend = XR_ENVIRONMENT_BLEND_MODE_OPAQUE;
static bool g_alphaSupported = false;  // runtime listed ALPHA_BLEND
static bool g_dpad[4] = {false, false, false, false};  // up, down, left, right
XrSessionState g_state = XR_SESSION_STATE_UNKNOWN;
bool g_sessionRunning = false;
bool g_quit = false;
bool g_frameBegun = false;
bool g_imagesAcquired[2] = {false, false};

XrSwapchain g_swapchain[2] = {XR_NULL_HANDLE, XR_NULL_HANDLE};
uint32_t g_swapW = 0, g_swapH = 0;
int64_t g_swapFormat = 0;
XrSwapchainImageOpenGLKHR g_images[2][kMaxSwapchainImages];
uint32_t g_imageCount[2] = {0, 0};
uint32_t g_imageIndex[2] = {0, 0};
XrCompositionLayerProjectionView g_projectionViews[2];
XrView g_views[2];

PFN_xrGetOpenGLGraphicsRequirementsKHR g_getGLRequirements = nullptr;

// Input. SteamVR only feeds controller buttons to the app that owns the
// session's action bindings, so the OpenVR background client sees poses but
// never a trigger; these actions are what actually deliver input.
XrActionSet g_actionSet = XR_NULL_HANDLE;

struct HandActions {
    XrAction triggerValue = XR_NULL_HANDLE;
    XrAction triggerClick = XR_NULL_HANDLE;
    XrAction stick = XR_NULL_HANDLE;
    XrAction stickClick = XR_NULL_HANDLE;
    XrAction a = XR_NULL_HANDLE;
    XrAction b = XR_NULL_HANDLE;
    XrAction x = XR_NULL_HANDLE;
    XrAction y = XR_NULL_HANDLE;
    XrAction gripPose = XR_NULL_HANDLE;
    XrAction aimPose = XR_NULL_HANDLE;
    XrAction haptic = XR_NULL_HANDLE;
    // Left d-pad: any direction toggles passthrough.
    XrAction dpadUp = XR_NULL_HANDLE;
    XrAction dpadDown = XR_NULL_HANDLE;
    XrAction dpadLeft = XR_NULL_HANDLE;
    XrAction dpadRight = XR_NULL_HANDLE;
};

HandActions g_actions;
XrSpace g_gripSpace[2] = {XR_NULL_HANDLE, XR_NULL_HANDLE};
XrSpace g_aimSpace[2] = {XR_NULL_HANDLE, XR_NULL_HANDLE};
XrPath g_handPath[2] = {XR_NULL_PATH, XR_NULL_PATH};
bool g_actionsAttached = false;
float g_triggerValue[2] = {0, 0};
bool g_triggerActive[2] = {false, false};
bool g_stickActive[2] = {false, false};
bool g_triggerClick[2] = {false, false};
XrVector2f g_stick[2] = {{0, 0}, {0, 0}};
bool g_stickClick[2] = {false, false};
bool g_buttonAbxy[2][4] = {{false, false, false, false}, {false, false, false, false}};
XrPosef g_gripPose[2];
XrPosef g_aimPose[2];
bool g_gripValid[2] = {false, false};
bool g_aimValid[2] = {false, false};

void setErr(char *err, int errlen, const char *what, XrResult r) {
    if (err && errlen > 0) snprintf(err, errlen, "%s -> %d", what, (int)r);
}

// Picks the fbconfig whose visual matches the window's so the GL context and
// the OpenXR binding agree on the drawable's format.
GLXFBConfig fbConfigForVisual(Display *dpy, VisualID visual, int screen) {
    int count = 0;
    GLXFBConfig *configs = glXGetFBConfigs(dpy, screen, &count);
    if (!configs) return nullptr;
    GLXFBConfig found = nullptr;
    for (int i = 0; i < count && !found; i++) {
        XVisualInfo *vi = glXGetVisualFromFBConfig(dpy, configs[i]);
        if (vi && vi->visualid == visual) found = configs[i];
        if (vi) XFree(vi);
    }
    XFree(configs);
    return found;
}

bool hasExtension(const char *name) {
    uint32_t count = 0;
    if (XR_FAILED(xrEnumerateInstanceExtensionProperties(nullptr, 0, &count, nullptr))) return false;
    std::vector<XrExtensionProperties> props(count, XrExtensionProperties{XR_TYPE_EXTENSION_PROPERTIES});
    if (XR_FAILED(xrEnumerateInstanceExtensionProperties(nullptr, count, &count, props.data()))) return false;
    for (uint32_t i = 0; i < count; i++)
        if (strcmp(props[i].extensionName, name) == 0) return true;
    return false;
}

XrResult createInstance(Display *dpy, XrInstance *out) {
    XrInstanceCreateInfo ci;
    memset(&ci, 0, sizeof ci);
    ci.type = XR_TYPE_INSTANCE_CREATE_INFO;
    snprintf(ci.applicationInfo.applicationName, XR_MAX_APPLICATION_NAME_SIZE, "zuma");
    ci.applicationInfo.applicationVersion = 1;
    snprintf(ci.applicationInfo.engineName, XR_MAX_ENGINE_NAME_SIZE, "zuma");
    ci.applicationInfo.engineVersion = 0;
    // The loader headers are newer than the runtime: 1.1 is rejected with
    // XR_ERROR_API_VERSION_UNSUPPORTED, 1.0 is what SteamVR implements.
    ci.applicationInfo.apiVersion = XR_MAKE_VERSION(1, 0, 0);
    // Graphics binding, the Frame Controller profile, and the render-model
    // extensions that hand us the controller meshes. XR_EXT_render_model needs
    // XR_EXT_uuid enabled alongside it or the loader rejects the instance with
    // XR_ERROR_VALIDATION_FAILURE.
    const char *exts[5];
    int extCount = 0;
    exts[extCount++] = kGLXExtension;
    if (hasExtension(kFrameControllerExtension)) {
        exts[extCount++] = kFrameControllerExtension;
        fprintf(stderr, "xr: frame controller interaction profile enabled\n");
    }
    if (hasExtension("XR_EXT_uuid") && hasExtension("XR_EXT_render_model") &&
        hasExtension("XR_EXT_interaction_render_model")) {
        exts[extCount++] = "XR_EXT_uuid";
        exts[extCount++] = "XR_EXT_render_model";
        exts[extCount++] = "XR_EXT_interaction_render_model";
        fprintf(stderr, "xr: controller render models enabled\n");
    }
    ci.enabledExtensionCount = static_cast<uint32_t>(extCount);
    ci.enabledExtensionNames = exts;
    return xrCreateInstance(&ci, out);
}

XrResult pickBlendMode(XrInstance instance, XrSystemId system) {
    uint32_t count = 0;
    XrResult r = xrEnumerateEnvironmentBlendModes(instance, system,
                                                 XR_VIEW_CONFIGURATION_TYPE_PRIMARY_STEREO, 0, &count,
                                                 nullptr);
    if (XR_FAILED(r) || count == 0) return r;
    std::vector<XrEnvironmentBlendMode> modes(count);
    r = xrEnumerateEnvironmentBlendModes(instance, system, XR_VIEW_CONFIGURATION_TYPE_PRIMARY_STEREO,
                                         count, &count, modes.data());
    if (XR_FAILED(r)) return r;
    g_blend = modes[0];
    for (uint32_t i = 0; i < count; i++)
        if (modes[i] == XR_ENVIRONMENT_BLEND_MODE_ALPHA_BLEND) {
            g_blend = XR_ENVIRONMENT_BLEND_MODE_ALPHA_BLEND;
            g_alphaSupported = true;
        }
    return XR_SUCCESS;
}

XrResult pickSwapchainFormat(XrSession session, int64_t *out) {
    uint32_t count = 0;
    XrResult r = xrEnumerateSwapchainFormats(session, 0, &count, nullptr);
    if (XR_FAILED(r) || count == 0) return r;
    std::vector<int64_t> formats(count);
    r = xrEnumerateSwapchainFormats(session, count, &count, formats.data());
    if (XR_FAILED(r) || count == 0) return r;
    int64_t chosen = formats[0];
    for (uint32_t i = 0; i < count; i++) {
        if (formats[i] == GL_RGBA8) {
            chosen = formats[i];
            break;
        }
        if (formats[i] == GL_SRGB8_ALPHA8) chosen = formats[i];
    }
    *out = chosen;
    return XR_SUCCESS;
}

// Creates the space the game works in. STAGE is the one whose origin sits on
// the floor, which is what "table height" means here; LOCAL is anchored at the
// headset, so using it puts a 0.5 m table at eye level.
XrResult createBaseSpace(XrSession session, XrSpace *out) {
    uint32_t count = 0;
    XrResult r = xrEnumerateReferenceSpaces(session, 0, &count, nullptr);
    if (XR_SUCCEEDED(r) && count > 0) {
        std::vector<XrReferenceSpaceType> types(count);
        uint32_t used = 0;
        if (XR_SUCCEEDED(xrEnumerateReferenceSpaces(session, count, &used, types.data()))) {
            for (uint32_t i = 0; i < used; i++)
                fprintf(stderr, "xr: reference space %d\n", (int)types[i]);
        }
    }

    XrReferenceSpaceCreateInfo ci;
    memset(&ci, 0, sizeof ci);
    ci.type = XR_TYPE_REFERENCE_SPACE_CREATE_INFO;
    ci.poseInReferenceSpace.orientation.w = 1.0f;

    // STAGE follows the play area exactly once. LOCAL_FLOOR looked like the
    // "raw" frame but SteamVR recentring moved it twice, so it is only a
    // fallback here.
    ci.referenceSpaceType = XR_REFERENCE_SPACE_TYPE_STAGE;
    r = xrCreateReferenceSpace(session, &ci, out);
    if (XR_SUCCEEDED(r)) {
        fprintf(stderr, "xr: using STAGE space (floor origin)\n");
        return r;
    }
    fprintf(stderr, "xr: STAGE space -> %d, falling back to LOCAL\n", (int)r);
    ci.referenceSpaceType = XR_REFERENCE_SPACE_TYPE_LOCAL;
    return xrCreateReferenceSpace(session, &ci, out);
}

// Creates both eye swapchains plus their GL images. The renderer draws into
// these textures straight through a framebuffer of its own.
XrResult createSwapchains(XrSession session, uint32_t w, uint32_t h) {
    int64_t format = 0;
    XrResult r = pickSwapchainFormat(session, &format);
    if (XR_FAILED(r)) return r;

    g_swapFormat = format;
    g_swapW = w;
    g_swapH = h;
    for (int eye = 0; eye < 2; eye++) {
        XrSwapchainCreateInfo ci;
        memset(&ci, 0, sizeof ci);
        ci.type = XR_TYPE_SWAPCHAIN_CREATE_INFO;
        ci.usageFlags = XR_SWAPCHAIN_USAGE_COLOR_ATTACHMENT_BIT;
        ci.format = format;
        ci.sampleCount = 1;
        ci.width = w;
        ci.height = h;
        ci.faceCount = 1;
        ci.arraySize = 1;
        ci.mipCount = 1;
        r = xrCreateSwapchain(session, &ci, &g_swapchain[eye]);
        if (XR_FAILED(r)) return r;

        uint32_t count = 0;
        r = xrEnumerateSwapchainImages(g_swapchain[eye], 0, &count, nullptr);
        if (XR_FAILED(r) || count == 0) return XR_ERROR_RUNTIME_FAILURE;
        if (count > (uint32_t)kMaxSwapchainImages) count = kMaxSwapchainImages;
        for (uint32_t i = 0; i < count; i++)
            g_images[eye][i] = XrSwapchainImageOpenGLKHR{XR_TYPE_SWAPCHAIN_IMAGE_OPENGL_KHR};
        r = xrEnumerateSwapchainImages(g_swapchain[eye], count, &count,
                                       reinterpret_cast<XrSwapchainImageBaseHeader *>(g_images[eye]));
        if (XR_FAILED(r)) return r;
        g_imageCount[eye] = count;
    }
    return XR_SUCCESS;
}

// Identity orientation with the given translation, as a row-major 3x4 matrix
// (OpenVR's layout, converted in Go).
void poseToMat34(const XrPosef &pose, float *out12) {
    const XrQuaternionf &q = pose.orientation;
    float x = q.x, y = q.y, z = q.z, w = q.w;
    float m[3][3] = {
        {1 - 2 * (y * y + z * z), 2 * (x * y - z * w), 2 * (x * z + y * w)},
        {2 * (x * y + z * w), 1 - 2 * (x * x + z * z), 2 * (y * z - x * w)},
        {2 * (x * z - y * w), 2 * (y * z + x * w), 1 - 2 * (x * x + y * y)},
    };
    for (int r = 0; r < 3; r++) {
        for (int c = 0; c < 3; c++) out12[r * 4 + c] = m[r][c];
    }
    out12[3] = pose.position.x;
    out12[7] = pose.position.y;
    out12[11] = pose.position.z;
}


XrAction makeAction(XrActionSet set, XrActionType type, const char *name) {
    XrActionCreateInfo ci;
    memset(&ci, 0, sizeof ci);
    ci.type = XR_TYPE_ACTION_CREATE_INFO;
    ci.actionType = type;
    snprintf(ci.actionName, XR_MAX_ACTION_NAME_SIZE, "%s", name);
    snprintf(ci.localizedActionName, XR_MAX_LOCALIZED_ACTION_NAME_SIZE, "%s", name);
    ci.countSubactionPaths = 2;
    ci.subactionPaths = g_handPath;
    XrAction action = XR_NULL_HANDLE;
    if (XR_FAILED(xrCreateAction(set, &ci, &action))) return XR_NULL_HANDLE;
    return action;
}

// Suggests one profile at a time. SteamVR implements touch, index and the KHR
// simple controller; the touch layout is the closest match to the frame
// controller's sticks and face buttons, so it goes first.
void suggestBindings() {
    struct Profile {
        const char *path;
        bool faceXY;       // profile has X/Y (touch) as well as A/B
        bool simple;       // KHR simple controller: select + poses only
        bool faceRightOnly; // frame controller: A/B/X/Y live on the right hand only
    };
    static const Profile profiles[] = {
        {kFrameControllerProfile, true, false, true},
        {"/interaction_profiles/oculus/touch_controller", true, false, false},
        {"/interaction_profiles/valve/index_controller", false, false, false},
        {"/interaction_profiles/khr/simple_controller", false, true, false},
    };

    for (const Profile &profile : profiles) {
        XrPath profilePath = XR_NULL_PATH;
        if (XR_FAILED(xrStringToPath(g_instance, profile.path, &profilePath))) continue;

        std::vector<XrActionSuggestedBinding> list;
        char buf[128];
        for (int hand = 0; hand < 2; hand++) {
            const char *side = hand == 1 ? "right" : "left";
            auto add = [&](XrAction action, const char *suffix) {
                snprintf(buf, sizeof buf, "/user/hand/%s/input/%s", side, suffix);
                XrPath path = XR_NULL_PATH;
                if (XR_FAILED(xrStringToPath(g_instance, buf, &path))) return;
                list.push_back(XrActionSuggestedBinding{action, path});
            };
            snprintf(buf, sizeof buf, "/user/hand/%s/output/haptic", side);
            XrPath hapticPath = XR_NULL_PATH;
            if (XR_SUCCEEDED(xrStringToPath(g_instance, buf, &hapticPath)))
                list.push_back(XrActionSuggestedBinding{g_actions.haptic, hapticPath});

            if (profile.simple) {
                add(g_actions.triggerClick, "select/click");
            } else {
                add(g_actions.triggerValue, "trigger/value");
                add(g_actions.triggerClick, "trigger/click");
                add(g_actions.stick, "thumbstick");
                add(g_actions.stickClick, "thumbstick/click");
                if (hand == 0) {
                    // The frame controller d-pad only exists on the left hand;
                    // any of its four directions toggles passthrough.
                    add(g_actions.dpadUp, "dpad_up/click");
                    add(g_actions.dpadDown, "dpad_down/click");
                    add(g_actions.dpadLeft, "dpad_left/click");
                    add(g_actions.dpadRight, "dpad_right/click");
                }
                if (!profile.faceRightOnly || hand == 1) {
                    add(g_actions.a, "a/click");
                    add(g_actions.b, "b/click");
                    if (profile.faceXY) {
                        add(g_actions.x, "x/click");
                        add(g_actions.y, "y/click");
                    }
                }
            }
            add(g_actions.gripPose, "grip/pose");
            add(g_actions.aimPose, "aim/pose");
        }

        XrInteractionProfileSuggestedBinding sb;
        memset(&sb, 0, sizeof sb);
        sb.type = XR_TYPE_INTERACTION_PROFILE_SUGGESTED_BINDING;
        sb.interactionProfile = profilePath;
        sb.countSuggestedBindings = (uint32_t)list.size();
        sb.suggestedBindings = list.data();
        XrResult r = xrSuggestInteractionProfileBindings(g_instance, &sb);
        fprintf(stderr, "xr: bindings %s -> %d (%zu paths)\n", profile.path, (int)r, list.size());
    }
}

bool setupActions(XrSession session) {
    for (int hand = 0; hand < 2; hand++) {
        const char *path = hand == 1 ? "/user/hand/right" : "/user/hand/left";
        if (XR_FAILED(xrStringToPath(g_instance, path, &g_handPath[hand]))) return false;
    }

    XrActionSetCreateInfo asci;
    memset(&asci, 0, sizeof asci);
    asci.type = XR_TYPE_ACTION_SET_CREATE_INFO;
    snprintf(asci.actionSetName, XR_MAX_ACTION_SET_NAME_SIZE, "game");
    snprintf(asci.localizedActionSetName, XR_MAX_LOCALIZED_ACTION_SET_NAME_SIZE, "Zuma");
    asci.priority = 0;
    if (XR_FAILED(xrCreateActionSet(g_instance, &asci, &g_actionSet))) return false;

    // One action per control, with both hand paths as subactions: names must be
    // unique inside the set, so creating a second copy per hand is an error.
    g_actions.triggerValue = makeAction(g_actionSet, XR_ACTION_TYPE_FLOAT_INPUT, "trigger");
    g_actions.triggerClick = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "trigger_click");
    g_actions.stick = makeAction(g_actionSet, XR_ACTION_TYPE_VECTOR2F_INPUT, "stick");
    g_actions.stickClick = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "stick_click");
    g_actions.a = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "a");
    g_actions.b = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "b");
    g_actions.x = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "x");
    g_actions.y = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "y");
    g_actions.gripPose = makeAction(g_actionSet, XR_ACTION_TYPE_POSE_INPUT, "grip_pose");
    g_actions.aimPose = makeAction(g_actionSet, XR_ACTION_TYPE_POSE_INPUT, "aim_pose");
    g_actions.haptic = makeAction(g_actionSet, XR_ACTION_TYPE_VIBRATION_OUTPUT, "haptic");
    g_actions.dpadUp = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "dpad_up");
    g_actions.dpadDown = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "dpad_down");
    g_actions.dpadLeft = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "dpad_left");
    g_actions.dpadRight = makeAction(g_actionSet, XR_ACTION_TYPE_BOOLEAN_INPUT, "dpad_right");
    suggestBindings();

    XrSessionActionSetsAttachInfo attach;
    memset(&attach, 0, sizeof attach);
    attach.type = XR_TYPE_SESSION_ACTION_SETS_ATTACH_INFO;
    attach.countActionSets = 1;
    attach.actionSets = &g_actionSet;
    if (XR_FAILED(xrAttachSessionActionSets(session, &attach))) return false;
    g_actionsAttached = true;

    for (int hand = 0; hand < 2; hand++) {
        auto makeSpace = [&](XrAction action, XrSpace *out) {
            XrActionSpaceCreateInfo ci;
            memset(&ci, 0, sizeof ci);
            ci.type = XR_TYPE_ACTION_SPACE_CREATE_INFO;
            ci.action = action;
            ci.subactionPath = g_handPath[hand];
            ci.poseInActionSpace.orientation.w = 1.0f;
            return XR_SUCCEEDED(xrCreateActionSpace(session, &ci, out));
        };
        if (!makeSpace(g_actions.gripPose, &g_gripSpace[hand])) return false;
        if (!makeSpace(g_actions.aimPose, &g_aimSpace[hand])) return false;
    }
    return true;
}

}  // namespace

extern "C" {

int xr_init(void *xdisplay, unsigned long xid, char *err, int errlen) {
    Display *dpy = static_cast<Display *>(xdisplay);
    if (!dpy) {
        snprintf(err, errlen, "no X11 display");
        return 0;
    }
    if (!hasExtension(kGLXExtension)) {
        snprintf(err, errlen, "runtime lacks %s", kGLXExtension);
        return 0;
    }

    XrResult r = createInstance(dpy, &g_instance);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrCreateInstance", r);
        return 0;
    }

    XrSystemGetInfo sgi;
    memset(&sgi, 0, sizeof sgi);
    sgi.type = XR_TYPE_SYSTEM_GET_INFO;
    sgi.formFactor = XR_FORM_FACTOR_HEAD_MOUNTED_DISPLAY;
    r = xrGetSystem(g_instance, &sgi, &g_system);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrGetSystem", r);
        return 0;
    }

    r = pickBlendMode(g_instance, g_system);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrEnumerateEnvironmentBlendModes", r);
        return 0;
    }

    XrSystemProperties props;
    memset(&props, 0, sizeof props);
    props.type = XR_TYPE_SYSTEM_PROPERTIES;
    if (XR_SUCCEEDED(xrGetSystemProperties(g_instance, g_system, &props)))
        fprintf(stderr, "xr: system=%s vendor=%u blend=%d\n", props.systemName,
                props.vendorId, (int)g_blend);

    if (!g_getGLRequirements) {
        PFN_xrVoidFunction fn = nullptr;
        if (XR_SUCCEEDED(xrGetInstanceProcAddr(g_instance, "xrGetOpenGLGraphicsRequirementsKHR", &fn)))
            g_getGLRequirements = reinterpret_cast<PFN_xrGetOpenGLGraphicsRequirementsKHR>(fn);
    }
    if (g_getGLRequirements) {
        XrGraphicsRequirementsOpenGLKHR req;
        memset(&req, 0, sizeof req);
        req.type = XR_TYPE_GRAPHICS_REQUIREMENTS_OPENGL_KHR;
        r = g_getGLRequirements(g_instance, g_system, &req);
        if (XR_FAILED(r)) {
            setErr(err, errlen, "xrGetOpenGLGraphicsRequirementsKHR", r);
            return 0;
        }
    }

    // View configuration: two eyes, each with the runtime's recommended size.
    uint32_t viewCount = 0;
    r = xrEnumerateViewConfigurationViews(g_instance, g_system, XR_VIEW_CONFIGURATION_TYPE_PRIMARY_STEREO,
                                         0, &viewCount, nullptr);
    if (XR_FAILED(r) || viewCount < 2) {
        setErr(err, errlen, "xrEnumerateViewConfigurationViews(count)", XR_FAILED(r) ? r : XR_ERROR_RUNTIME_FAILURE);
        return 0;
    }
    XrViewConfigurationView views[2];
    memset(views, 0, sizeof views);
    views[0].type = XR_TYPE_VIEW_CONFIGURATION_VIEW;
    views[1].type = XR_TYPE_VIEW_CONFIGURATION_VIEW;
    viewCount = 2;
    r = xrEnumerateViewConfigurationViews(g_instance, g_system, XR_VIEW_CONFIGURATION_TYPE_PRIMARY_STEREO,
                                         2, &viewCount, views);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrEnumerateViewConfigurationViews", r);
        return 0;
    }

    XWindowAttributes wa;
    if (!XGetWindowAttributes(dpy, static_cast<Window>(xid), &wa)) {
        snprintf(err, errlen, "XGetWindowAttributes failed");
        return 0;
    }
    GLXFBConfig fbconfig = fbConfigForVisual(dpy, wa.visual->visualid, XScreenNumberOfScreen(wa.screen));
    if (!fbconfig) {
        snprintf(err, errlen, "no GLX fbconfig for the window visual");
        return 0;
    }
    GLXDrawable drawable = glXGetCurrentDrawable();
    GLXContext context = glXGetCurrentContext();
    if (!context) {
        snprintf(err, errlen, "no current GLX context");
        return 0;
    }

    XrGraphicsBindingOpenGLXlibKHR binding;
    memset(&binding, 0, sizeof binding);
    binding.type = XR_TYPE_GRAPHICS_BINDING_OPENGL_XLIB_KHR;
    binding.xDisplay = dpy;
    binding.visualid = wa.visual->visualid;
    binding.glxFBConfig = fbconfig;
    binding.glxDrawable = drawable;
    binding.glxContext = context;

    XrSessionCreateInfo sci;
    memset(&sci, 0, sizeof sci);
    sci.type = XR_TYPE_SESSION_CREATE_INFO;
    sci.next = &binding;
    sci.systemId = g_system;
    r = xrCreateSession(g_instance, &sci, &g_session);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrCreateSession", r);
        return 0;
    }

    r = createBaseSpace(g_session, &g_local);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrCreateReferenceSpace", r);
        return 0;
    }
    XrReferenceSpaceCreateInfo rsci;
    memset(&rsci, 0, sizeof rsci);
    rsci.type = XR_TYPE_REFERENCE_SPACE_CREATE_INFO;
    rsci.referenceSpaceType = XR_REFERENCE_SPACE_TYPE_VIEW;
    rsci.poseInReferenceSpace.orientation.w = 1.0f;
    r = xrCreateReferenceSpace(g_session, &rsci, &g_viewSpace);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrCreateReferenceSpace(VIEW)", r);
        return 0;
    }

    if (!setupActions(g_session)) {
        snprintf(err, errlen, "controller actions could not be created");
        return 0;
    }

    r = createSwapchains(g_session, views[0].recommendedImageRectWidth,
                         views[0].recommendedImageRectHeight);
    if (XR_FAILED(r)) {
        setErr(err, errlen, "xrCreateSwapchain", r);
        return 0;
    }

    g_state = XR_SESSION_STATE_UNKNOWN;
    g_quit = false;
    return 1;
}

void xr_shutdown(void) {
    // Once the runtime has asked us to quit (STOPPING/EXITING/LOSS_PENDING) it
    // has already torn the session down; calling the destroy chain there aborts
    // inside SteamVR's shared-texture teardown, and the process is leaving
    // anyway. Only unwind a session that is still live.
    if (g_state >= XR_SESSION_STATE_STOPPING) {
        g_session = XR_NULL_HANDLE;
        g_instance = XR_NULL_HANDLE;
        g_sessionRunning = false;
        return;
    }
    if (g_session != XR_NULL_HANDLE) {
        if (g_sessionRunning) {
            xrEndSession(g_session);
            g_sessionRunning = false;
        }
        for (int eye = 0; eye < 2; eye++) {
            if (g_swapchain[eye] != XR_NULL_HANDLE) {
                xrDestroySwapchain(g_swapchain[eye]);
                g_swapchain[eye] = XR_NULL_HANDLE;
            }
        }
        if (g_local != XR_NULL_HANDLE) xrDestroySpace(g_local);
        if (g_viewSpace != XR_NULL_HANDLE) xrDestroySpace(g_viewSpace);
        g_local = g_viewSpace = XR_NULL_HANDLE;
        xrDestroySession(g_session);
        g_session = XR_NULL_HANDLE;
    }
    if (g_instance != XR_NULL_HANDLE) {
        xrDestroyInstance(g_instance);
        g_instance = XR_NULL_HANDLE;
    }
    g_blend = XR_ENVIRONMENT_BLEND_MODE_OPAQUE;
    g_state = XR_SESSION_STATE_UNKNOWN;
}

int xr_blend_mode(void) { return (int)g_blend; }

unsigned int xr_swapchain_format(void) { return (unsigned int)g_swapFormat; }

int xr_state(void) { return (int)g_state; }

void xr_size(int *w, int *h) {
    if (w) *w = (int)g_swapW;
    if (h) *h = (int)g_swapH;
}

int xr_poll(void) {
    if (g_instance == XR_NULL_HANDLE) return 1;
    XrEventDataBuffer ev;
    memset(&ev, 0, sizeof ev);
    ev.type = XR_TYPE_EVENT_DATA_BUFFER;
    while (xrPollEvent(g_instance, &ev) == XR_SUCCESS) {
        if (ev.type == XR_TYPE_EVENT_DATA_SESSION_STATE_CHANGED) {
            const XrEventDataSessionStateChanged *sc =
                reinterpret_cast<const XrEventDataSessionStateChanged *>(&ev);
            g_state = sc->state;
            fprintf(stderr, "xr: session state -> %d\n", (int)g_state);
            switch (g_state) {
                case XR_SESSION_STATE_READY: {
                    if (!g_sessionRunning) {
                        XrSessionBeginInfo bi;
                        memset(&bi, 0, sizeof bi);
                        bi.type = XR_TYPE_SESSION_BEGIN_INFO;
                        bi.primaryViewConfigurationType = XR_VIEW_CONFIGURATION_TYPE_PRIMARY_STEREO;
                        XrResult r = xrBeginSession(g_session, &bi);
                        if (XR_SUCCEEDED(r)) {
                            g_sessionRunning = true;
                        } else {
                            fprintf(stderr, "xr: xrBeginSession -> %d\n", (int)r);
                        }
                    }
                    break;
                }
                case XR_SESSION_STATE_STOPPING:
                    if (g_sessionRunning) {
                        xrEndSession(g_session);
                        g_sessionRunning = false;
                    }
                    break;
                case XR_SESSION_STATE_EXITING:
                case XR_SESSION_STATE_LOSS_PENDING:
                    g_quit = true;
                    break;
                default:
                    break;
            }
        } else if (ev.type == XR_TYPE_EVENT_DATA_INTERACTION_PROFILE_CHANGED) {
            // Which profile the runtime actually bound: the frame controller one
            // means the native A/B/X/Y paths are live instead of the index
            // compatibility layer.
            for (int hand = 0; hand < 2; hand++) {
                if (g_handPath[hand] == XR_NULL_PATH) continue;
                XrInteractionProfileState st;
                memset(&st, 0, sizeof st);
                st.type = XR_TYPE_INTERACTION_PROFILE_STATE;
                if (XR_FAILED(xrGetCurrentInteractionProfile(g_session, g_handPath[hand], &st))) continue;
                if (st.interactionProfile == XR_NULL_PATH) continue;
                char path[XR_MAX_PATH_LENGTH] = {0};
                uint32_t len = 0;
                if (XR_SUCCEEDED(
                        xrPathToString(g_instance, st.interactionProfile, sizeof path, &len, path)))
                    fprintf(stderr, "xr: %s profile %s\n", hand == 1 ? "right" : "left", path);
            }
        } else if (ev.type == XR_TYPE_EVENT_DATA_INSTANCE_LOSS_PENDING) {
            g_quit = true;
        }
        memset(&ev, 0, sizeof ev);
        ev.type = XR_TYPE_EVENT_DATA_BUFFER;
    }
    return g_quit ? 1 : 0;
}

int xr_wait_frame(double *out_time) {
    if (g_session == XR_NULL_HANDLE) return 0;
    XrFrameWaitInfo wi;
    memset(&wi, 0, sizeof wi);
    wi.type = XR_TYPE_FRAME_WAIT_INFO;
    XrFrameState fs;
    memset(&fs, 0, sizeof fs);
    fs.type = XR_TYPE_FRAME_STATE;
    XrResult r = xrWaitFrame(g_session, &wi, &fs);
    if (XR_FAILED(r)) return 0;

    XrFrameBeginInfo bi;
    memset(&bi, 0, sizeof bi);
    bi.type = XR_TYPE_FRAME_BEGIN_INFO;
    r = xrBeginFrame(g_session, &bi);
    if (XR_FAILED(r)) return 0;
    g_frameBegun = true;
    g_imagesAcquired[0] = g_imagesAcquired[1] = false;

    bool visible = g_state == XR_SESSION_STATE_VISIBLE || g_state == XR_SESSION_STATE_FOCUSED;
    if (!fs.shouldRender || !visible) {
        XrFrameEndInfo ei;
        memset(&ei, 0, sizeof ei);
        ei.type = XR_TYPE_FRAME_END_INFO;
        ei.displayTime = fs.predictedDisplayTime;
        ei.environmentBlendMode = g_blend;
        ei.layerCount = 0;
        xrEndFrame(g_session, &ei);
        g_frameBegun = false;
        return 0;
    }
    if (out_time) *out_time = (double)fs.predictedDisplayTime / 1e9;
    return 1;
}

int xr_locate_views(double display_time) {
    if (g_session == XR_NULL_HANDLE) return 0;
    XrViewLocateInfo li;
    memset(&li, 0, sizeof li);
    li.type = XR_TYPE_VIEW_LOCATE_INFO;
    li.viewConfigurationType = XR_VIEW_CONFIGURATION_TYPE_PRIMARY_STEREO;
    li.displayTime = (XrTime)(display_time * 1e9);
    li.space = g_local;
    XrViewState vs;
    memset(&vs, 0, sizeof vs);
    vs.type = XR_TYPE_VIEW_STATE;
    uint32_t count = 2;
    for (int eye = 0; eye < 2; eye++) memset(&g_views[eye], 0, sizeof g_views[eye]);
    g_views[0].type = XR_TYPE_VIEW;
    g_views[1].type = XR_TYPE_VIEW;
    XrResult r = xrLocateViews(g_session, &li, &vs, 2, &count, g_views);
    if (XR_FAILED(r) || count < 2) return 0;
    return 1;
}

void xr_eye_pose(int eye, float *out12) {
    if (eye < 0 || eye > 1 || !out12) return;
    poseToMat34(g_views[eye].pose, out12);
}

void xr_eye_fov(int eye, float *out4) {
    if (eye < 0 || eye > 1 || !out4) return;
    out4[0] = g_views[eye].fov.angleLeft;
    out4[1] = g_views[eye].fov.angleRight;
    out4[2] = g_views[eye].fov.angleUp;
    out4[3] = g_views[eye].fov.angleDown;
}

int xr_head_pose(double display_time, float *out12) {
    if (g_viewSpace == XR_NULL_HANDLE) return 0;
    XrSpaceLocation loc;
    memset(&loc, 0, sizeof loc);
    loc.type = XR_TYPE_SPACE_LOCATION;
    XrResult r = xrLocateSpace(g_viewSpace, g_local, (XrTime)(display_time * 1e9), &loc);
    if (XR_FAILED(r)) return 0;
    if (!(loc.locationFlags & XR_SPACE_LOCATION_ORIENTATION_VALID_BIT) ||
        !(loc.locationFlags & XR_SPACE_LOCATION_POSITION_VALID_BIT))
        return 0;
    poseToMat34(loc.pose, out12);
    return 1;
}

int xr_acquire(int eye, unsigned int *texture) {
    if (eye < 0 || eye > 1 || !g_frameBegun || g_swapchain[eye] == XR_NULL_HANDLE) return 0;
    XrSwapchainImageAcquireInfo ai;
    memset(&ai, 0, sizeof ai);
    ai.type = XR_TYPE_SWAPCHAIN_IMAGE_ACQUIRE_INFO;
    uint32_t index = 0;
    if (XR_FAILED(xrAcquireSwapchainImage(g_swapchain[eye], &ai, &index))) return 0;
    XrSwapchainImageWaitInfo wi;
    memset(&wi, 0, sizeof wi);
    wi.type = XR_TYPE_SWAPCHAIN_IMAGE_WAIT_INFO;
    wi.timeout = XR_INFINITE_DURATION;
    if (XR_FAILED(xrWaitSwapchainImage(g_swapchain[eye], &wi))) return 0;
    g_imageIndex[eye] = index;
    g_imagesAcquired[eye] = true;
    if (texture) *texture = g_images[eye][index].image;
    return 1;
}

int xr_sync_input(double display_time) {
    if (!g_actionsAttached || g_session == XR_NULL_HANDLE) return 0;
    XrActiveActionSet active;
    memset(&active, 0, sizeof active);
    active.actionSet = g_actionSet;
    active.subactionPath = XR_NULL_PATH;
    XrActionsSyncInfo si;
    memset(&si, 0, sizeof si);
    si.type = XR_TYPE_ACTIONS_SYNC_INFO;
    si.countActiveActionSets = 1;
    si.activeActionSets = &active;
    if (XR_FAILED(xrSyncActions(g_session, &si))) return 0;

    const XrTime when = (XrTime)(display_time * 1e9);

    for (int hand = 0; hand < 2; hand++) {
        const HandActions &ha = g_actions;
        auto stateInfo = [&](XrAction action) {
            XrActionStateGetInfo gi;
            memset(&gi, 0, sizeof gi);
            gi.type = XR_TYPE_ACTION_STATE_GET_INFO;
            gi.action = action;
            gi.subactionPath = g_handPath[hand];
            return gi;
        };

        XrActionStateGetInfo gi = stateInfo(ha.triggerValue);
        XrActionStateFloat f;
        memset(&f, 0, sizeof f);
        f.type = XR_TYPE_ACTION_STATE_FLOAT;
        const bool triggerOK = XR_SUCCEEDED(xrGetActionStateFloat(g_session, &gi, &f)) && f.isActive;
        g_triggerActive[hand] = triggerOK;
        g_triggerValue[hand] = triggerOK ? f.currentState : 0.0f;

        auto readBool = [&](XrAction action) {
            XrActionStateGetInfo bi = stateInfo(action);
            XrActionStateBoolean b;
            memset(&b, 0, sizeof b);
            b.type = XR_TYPE_ACTION_STATE_BOOLEAN;
            return XR_SUCCEEDED(xrGetActionStateBoolean(g_session, &bi, &b)) && b.isActive &&
                   b.currentState;
        };
        g_triggerClick[hand] = readBool(ha.triggerClick);
        g_stickClick[hand] = readBool(ha.stickClick);
        g_buttonAbxy[hand][0] = readBool(ha.a);
        g_buttonAbxy[hand][1] = readBool(ha.b);
        g_buttonAbxy[hand][2] = readBool(ha.x);
        g_buttonAbxy[hand][3] = readBool(ha.y);
        if (hand == 0) {
            const bool u = readBool(ha.dpadUp);
            const bool d = readBool(ha.dpadDown);
            const bool l = readBool(ha.dpadLeft);
            const bool r = readBool(ha.dpadRight);
            if (u != g_dpad[0] || d != g_dpad[1] || l != g_dpad[2] || r != g_dpad[3])
                fprintf(stderr, "xr: dpad %d%d%d%d\n", u ? 1 : 0, d ? 1 : 0, l ? 1 : 0, r ? 1 : 0);
            g_dpad[0] = u;
            g_dpad[1] = d;
            g_dpad[2] = l;
            g_dpad[3] = r;
        }

        gi = stateInfo(ha.stick);
        XrActionStateVector2f v;
        memset(&v, 0, sizeof v);
        v.type = XR_TYPE_ACTION_STATE_VECTOR2F;
        const bool stickOK = XR_SUCCEEDED(xrGetActionStateVector2f(g_session, &gi, &v)) && v.isActive;
        g_stickActive[hand] = stickOK;
        g_stick[hand] = stickOK ? v.currentState : XrVector2f{0, 0};

        if (hand == 1) {
            static int ticks = 0;
            if (++ticks % 240 == 1) {
                fprintf(stderr, "xr: input left trig=%d stick=%d | right trig=%d stick=%d\n",
                        g_triggerActive[0] ? 1 : 0, g_stickActive[0] ? 1 : 0,
                        g_triggerActive[1] ? 1 : 0, g_stickActive[1] ? 1 : 0);
            }
        }

        auto locate = [&](XrSpace space, XrPosef *out, bool *valid) {
            XrSpaceLocation loc;
            memset(&loc, 0, sizeof loc);
            loc.type = XR_TYPE_SPACE_LOCATION;
            *valid = false;
            if (space == XR_NULL_HANDLE) return;
            if (XR_FAILED(xrLocateSpace(space, g_local, when, &loc))) return;
            if (!(loc.locationFlags & XR_SPACE_LOCATION_ORIENTATION_VALID_BIT) ||
                !(loc.locationFlags & XR_SPACE_LOCATION_POSITION_VALID_BIT))
                return;
            *out = loc.pose;
            *valid = true;
        };
        locate(g_gripSpace[hand], &g_gripPose[hand], &g_gripValid[hand]);
        locate(g_aimSpace[hand], &g_aimPose[hand], &g_aimValid[hand]);
    }
    return 1;
}

float xr_hand_trigger(int hand) {
    return (hand == 0 || hand == 1) ? g_triggerValue[hand] : 0.0f;
}

int xr_hand_trigger_click(int hand) {
    return (hand == 0 || hand == 1) ? (g_triggerClick[hand] ? 1 : 0) : 0;
}

int xr_dpad_any(void) {
    return (g_dpad[0] || g_dpad[1] || g_dpad[2] || g_dpad[3]) ? 1 : 0;
}

// xr_set_passthrough switches the environment blend mode at runtime: alpha
// blend shows the headset cameras wherever the app leaves pixels transparent.
void xr_set_passthrough(int on) {
    if (on && g_alphaSupported) {
        g_blend = XR_ENVIRONMENT_BLEND_MODE_ALPHA_BLEND;
    } else {
        g_blend = XR_ENVIRONMENT_BLEND_MODE_OPAQUE;
    }
}

void xr_hand_stick(int hand, float *x, float *y) {
    if (hand != 0 && hand != 1) {
        if (x) *x = 0;
        if (y) *y = 0;
        return;
    }
    if (x) *x = g_stick[hand].x;
    if (y) *y = g_stick[hand].y;
}

int xr_hand_buttons(int hand) {
    if (hand != 0 && hand != 1) return 0;
    int bits = 0;
    if (g_buttonAbxy[hand][0]) bits |= 1;
    if (g_buttonAbxy[hand][1]) bits |= 2;
    if (g_buttonAbxy[hand][2]) bits |= 4;
    if (g_buttonAbxy[hand][3]) bits |= 8;
    if (g_stickClick[hand]) bits |= 16;
    return bits;
}

int xr_hand_pose(int hand, int aim, float *out12) {
    if ((hand != 0 && hand != 1) || !out12) return 0;
    const bool valid = aim ? g_aimValid[hand] : g_gripValid[hand];
    if (!valid) return 0;
    poseToMat34(aim ? g_aimPose[hand] : g_gripPose[hand], out12);
    return 1;
}

int xr_haptic(int hand, float amplitude) {
    if ((hand != 0 && hand != 1) || !g_actionsAttached) return 0;
    if (amplitude <= 0.0f) return 1;
    XrHapticVibration vibration;
    memset(&vibration, 0, sizeof vibration);
    vibration.type = XR_TYPE_HAPTIC_VIBRATION;
    vibration.amplitude = amplitude > 1.0f ? 1.0f : amplitude;
    vibration.duration = XR_MIN_HAPTIC_DURATION;
    vibration.frequency = XR_FREQUENCY_UNSPECIFIED;
    XrHapticActionInfo hi;
    memset(&hi, 0, sizeof hi);
    hi.type = XR_TYPE_HAPTIC_ACTION_INFO;
    hi.action = g_actions.haptic;
    hi.subactionPath = g_handPath[hand];
    return XR_SUCCEEDED(xrApplyHapticFeedback(g_session, &hi,
                                             reinterpret_cast<const XrHapticBaseHeader *>(&vibration)))
               ? 1
               : 0;
}


// --- interaction render models (XR_EXT_render_model) ----------------------

static XrRenderModelEXT g_model[2] = {XR_NULL_HANDLE, XR_NULL_HANDLE};
static XrSpace g_modelSpace[2] = {XR_NULL_HANDLE, XR_NULL_HANDLE};
//
// The runtime owns the controller meshes: an app can ask for the glTF asset of
// the model attached to one hand. That replaces SteamVR's IVRRenderModels
// without adding an OpenVR client to the process.

static PFN_xrEnumerateInteractionRenderModelIdsEXT g_pModelIds = nullptr;
static PFN_xrEnumerateRenderModelSubactionPathsEXT g_pModelPaths = nullptr;
static PFN_xrCreateRenderModelEXT g_pCreateModel = nullptr;
static PFN_xrDestroyRenderModelEXT g_pDestroyModel = nullptr;
static PFN_xrGetRenderModelPropertiesEXT g_pModelProps = nullptr;
static PFN_xrCreateRenderModelAssetEXT g_pCreateAsset = nullptr;
static PFN_xrDestroyRenderModelAssetEXT g_pDestroyAsset = nullptr;
static PFN_xrGetRenderModelAssetDataEXT g_pModelAssetData = nullptr;

static PFN_xrVoidFunction xrGetInstanceProcAddrWrapper(const char *name) {
    PFN_xrVoidFunction fn = nullptr;
    if (XR_FAILED(xrGetInstanceProcAddr(g_instance, name, &fn))) return nullptr;
    return fn;
}

static bool resolveModelProcs() {
    if (g_pModelIds != nullptr) return true;
    auto get = [](const char *name) -> PFN_xrVoidFunction {
        PFN_xrVoidFunction fn = nullptr;
        if (XR_FAILED(xrGetInstanceProcAddr(g_instance, name, &fn))) return nullptr;
        return fn;
    };
    g_pModelIds = reinterpret_cast<PFN_xrEnumerateInteractionRenderModelIdsEXT>(
        get("xrEnumerateInteractionRenderModelIdsEXT"));
    g_pModelPaths = reinterpret_cast<PFN_xrEnumerateRenderModelSubactionPathsEXT>(
        get("xrEnumerateRenderModelSubactionPathsEXT"));
    g_pCreateModel =
        reinterpret_cast<PFN_xrCreateRenderModelEXT>(get("xrCreateRenderModelEXT"));
    g_pDestroyModel =
        reinterpret_cast<PFN_xrDestroyRenderModelEXT>(get("xrDestroyRenderModelEXT"));
    g_pModelProps =
        reinterpret_cast<PFN_xrGetRenderModelPropertiesEXT>(get("xrGetRenderModelPropertiesEXT"));
    g_pCreateAsset =
        reinterpret_cast<PFN_xrCreateRenderModelAssetEXT>(get("xrCreateRenderModelAssetEXT"));
    g_pDestroyAsset =
        reinterpret_cast<PFN_xrDestroyRenderModelAssetEXT>(get("xrDestroyRenderModelAssetEXT"));
    g_pModelAssetData =
        reinterpret_cast<PFN_xrGetRenderModelAssetDataEXT>(get("xrGetRenderModelAssetDataEXT"));
    return g_pModelIds && g_pModelPaths && g_pCreateModel && g_pDestroyModel && g_pModelProps &&
           g_pCreateAsset && g_pDestroyAsset && g_pModelAssetData;
}

// True when this model belongs to the requested hand. Models carry no hand bit:
// the subaction paths they answer to do it.
static bool modelIsForHand(XrRenderModelEXT model, const char *want) {
    XrInteractionRenderModelSubactionPathInfoEXT info;
    memset(&info, 0, sizeof info);
    info.type = XR_TYPE_INTERACTION_RENDER_MODEL_SUBACTION_PATH_INFO_EXT;
    uint32_t count = 0;
    if (XR_FAILED(g_pModelPaths(model, &info, 0, &count, nullptr)) || count == 0) return false;
    std::vector<XrPath> paths(count);
    if (XR_FAILED(g_pModelPaths(model, &info, count, &count, paths.data()))) return false;
    for (uint32_t i = 0; i < count; i++) {
        char buf[XR_MAX_PATH_LENGTH] = {0};
        uint32_t len = 0;
        if (XR_FAILED(xrPathToString(g_instance, paths[i], sizeof buf, &len, buf))) continue;
        if (strcmp(buf, want) == 0) return true;
    }
    return false;
}

// xr_model_pose reports the render model space of one hand inside that hand's
// grip space: the mesh vertices are authored in this frame.
int xr_model_pose(int hand, double display_time, float *out12) {
    if (hand < 0 || hand > 1 || out12 == nullptr) return 0;
    if (g_modelSpace[hand] == XR_NULL_HANDLE || g_gripSpace[hand] == XR_NULL_HANDLE) return 0;
    XrSpaceLocation loc;
    memset(&loc, 0, sizeof loc);
    loc.type = XR_TYPE_SPACE_LOCATION;
    XrResult r = xrLocateSpace(g_modelSpace[hand], g_gripSpace[hand],
                              (XrTime)(display_time * 1e9), &loc);
    if (XR_FAILED(r)) return 0;
    if ((loc.locationFlags & XR_SPACE_LOCATION_ORIENTATION_VALID_BIT) == 0 ||
        (loc.locationFlags & XR_SPACE_LOCATION_POSITION_VALID_BIT) == 0)
        return 0;
    poseToMat34(loc.pose, out12);
    return 1;
}

// xr_model_gltf copies the glTF asset of one hand's controller into a malloc'd
// buffer the caller frees with free(). Returns null and fills err on failure.
unsigned char *xr_model_gltf(int hand, int *out_len, char *err, int errlen) {
    if (out_len) *out_len = 0;
    if (g_session == XR_NULL_HANDLE) {
        snprintf(err, errlen, "no session");
        return nullptr;
    }
    if (!resolveModelProcs()) {
        snprintf(err, errlen, "runtime lacks XR_EXT_render_model");
        return nullptr;
    }

    XrInteractionRenderModelIdsEnumerateInfoEXT info;
    memset(&info, 0, sizeof info);
    info.type = XR_TYPE_INTERACTION_RENDER_MODEL_IDS_ENUMERATE_INFO_EXT;
    uint32_t count = 0;
    if (XR_FAILED(g_pModelIds(g_session, &info, 0, &count, nullptr)) || count == 0) {
        snprintf(err, errlen, "no interaction render models (bind a pose action first)");
        return nullptr;
    }
    std::vector<XrRenderModelIdEXT> ids(count);
    if (XR_FAILED(g_pModelIds(g_session, &info, count, &count, ids.data()))) {
        snprintf(err, errlen, "render model enumeration failed");
        return nullptr;
    }

    const char *want = hand == 1 ? "/user/hand/right" : "/user/hand/left";
    for (uint32_t i = 0; i < count; i++) {
        XrRenderModelCreateInfoEXT mci;
        memset(&mci, 0, sizeof mci);
        mci.type = XR_TYPE_RENDER_MODEL_CREATE_INFO_EXT;
        mci.renderModelId = ids[i];
        XrRenderModelEXT model = XR_NULL_HANDLE;
        if (XR_FAILED(g_pCreateModel(g_session, &mci, &model))) continue;
        if (!modelIsForHand(model, want)) {
            g_pDestroyModel(model);
            continue;
        }

        XrRenderModelPropertiesGetInfoEXT pgi;
        memset(&pgi, 0, sizeof pgi);
        pgi.type = XR_TYPE_RENDER_MODEL_PROPERTIES_GET_INFO_EXT;
        XrRenderModelPropertiesEXT props;
        memset(&props, 0, sizeof props);
        props.type = XR_TYPE_RENDER_MODEL_PROPERTIES_EXT;
        if (XR_FAILED(g_pModelProps(model, &pgi, &props))) {
            g_pDestroyModel(model);
            snprintf(err, errlen, "render model properties failed");
            return nullptr;
        }

        XrRenderModelAssetCreateInfoEXT aci;
        memset(&aci, 0, sizeof aci);
        aci.type = XR_TYPE_RENDER_MODEL_ASSET_CREATE_INFO_EXT;
        aci.cacheId = props.cacheId;
        XrRenderModelAssetEXT asset = XR_NULL_HANDLE;
        if (XR_FAILED(g_pCreateAsset(g_session, &aci, &asset))) {
            g_pDestroyModel(model);
            snprintf(err, errlen, "render model asset failed");
            return nullptr;
        }

        XrRenderModelAssetDataGetInfoEXT dgi;
        memset(&dgi, 0, sizeof dgi);
        dgi.type = XR_TYPE_RENDER_MODEL_ASSET_DATA_GET_INFO_EXT;
        XrRenderModelAssetDataEXT size;
        memset(&size, 0, sizeof size);
        size.type = XR_TYPE_RENDER_MODEL_ASSET_DATA_EXT;
        if (XR_FAILED(g_pModelAssetData(asset, &dgi, &size)) || size.bufferCountOutput == 0) {
            g_pDestroyAsset(asset);
            g_pDestroyModel(model);
            snprintf(err, errlen, "render model asset has no data");
            return nullptr;
        }
        unsigned char *buf = static_cast<unsigned char *>(malloc(size.bufferCountOutput));
        if (buf == nullptr) {
            g_pDestroyAsset(asset);
            g_pDestroyModel(model);
            snprintf(err, errlen, "out of memory (%u bytes)", size.bufferCountOutput);
            return nullptr;
        }
        XrRenderModelAssetDataEXT data;
        memset(&data, 0, sizeof data);
        data.type = XR_TYPE_RENDER_MODEL_ASSET_DATA_EXT;
        data.bufferCapacityInput = size.bufferCountOutput;
        data.buffer = buf;
        XrResult r = g_pModelAssetData(asset, &dgi, &data);
        g_pDestroyAsset(asset);
        if (XR_FAILED(r)) {
            g_pDestroyModel(model);
            free(buf);
            snprintf(err, errlen, "render model asset read failed");
            return nullptr;
        }
        // Keep the model: its space is what tells us where the mesh sits
        // relative to the grip pose, so the drawing code does not have to
        // guess an offset.
        XrRenderModelSpaceCreateInfoEXT sci2;
        memset(&sci2, 0, sizeof sci2);
        sci2.type = XR_TYPE_RENDER_MODEL_SPACE_CREATE_INFO_EXT;
        sci2.renderModel = model;
        PFN_xrCreateRenderModelSpaceEXT createSpace =
            reinterpret_cast<PFN_xrCreateRenderModelSpaceEXT>(
                xrGetInstanceProcAddrWrapper("xrCreateRenderModelSpaceEXT"));
        if (createSpace != nullptr &&
            XR_SUCCEEDED(createSpace(g_session, &sci2, &g_modelSpace[hand]))) {
            g_model[hand] = model;
        } else {
            g_pDestroyModel(model);
        }
        if (out_len) *out_len = static_cast<int>(data.bufferCountOutput);
        return buf;
    }

    snprintf(err, errlen, "no render model for %s", want);
    return nullptr;
}

int xr_end_frame(double display_time) {
    if (!g_frameBegun) return 0;
    for (int eye = 0; eye < 2; eye++) {
        if (g_imagesAcquired[eye]) {
            XrSwapchainImageReleaseInfo ri;
            memset(&ri, 0, sizeof ri);
            ri.type = XR_TYPE_SWAPCHAIN_IMAGE_RELEASE_INFO;
            xrReleaseSwapchainImage(g_swapchain[eye], &ri);
            g_imagesAcquired[eye] = false;
        }
        memset(&g_projectionViews[eye], 0, sizeof g_projectionViews[eye]);
        g_projectionViews[eye].type = XR_TYPE_COMPOSITION_LAYER_PROJECTION_VIEW;
        g_projectionViews[eye].pose = g_views[eye].pose;
        g_projectionViews[eye].fov = g_views[eye].fov;
        g_projectionViews[eye].subImage.swapchain = g_swapchain[eye];
        g_projectionViews[eye].subImage.imageRect.offset.x = 0;
        g_projectionViews[eye].subImage.imageRect.offset.y = 0;
        g_projectionViews[eye].subImage.imageRect.extent.width = (int32_t)g_swapW;
        g_projectionViews[eye].subImage.imageRect.extent.height = (int32_t)g_swapH;
        g_projectionViews[eye].subImage.imageArrayIndex = 0;
    }

    XrCompositionLayerProjection projection;
    memset(&projection, 0, sizeof projection);
    projection.type = XR_TYPE_COMPOSITION_LAYER_PROJECTION;
    projection.space = g_local;
    projection.viewCount = 2;
    projection.views = g_projectionViews;
    const XrCompositionLayerBaseHeader *layers[1] = {
        reinterpret_cast<const XrCompositionLayerBaseHeader *>(&projection)};

    XrFrameEndInfo ei;
    memset(&ei, 0, sizeof ei);
    ei.type = XR_TYPE_FRAME_END_INFO;
    ei.displayTime = (XrTime)(display_time * 1e9);
    ei.environmentBlendMode = g_blend;
    ei.layerCount = 1;
    ei.layers = layers;
    XrResult r = xrEndFrame(g_session, &ei);
    g_frameBegun = false;
    return XR_SUCCEEDED(r) ? 1 : 0;
}

}  // extern "C"
