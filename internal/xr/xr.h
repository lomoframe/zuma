#ifndef ZUMA_XR_H
#define ZUMA_XR_H

#ifdef __cplusplus
extern "C" {
#endif

/* Minimal OpenXR display layer: instance, GLX-bound session, one swapchain per
 * eye and a projection layer submitted with XR_ENVIRONMENT_BLEND_MODE_ALPHA_BLEND
 * when the runtime offers it.
 *
 * xdisplay is the X11 Display* of the GLFW window, xid its X window. The
 * calling thread must have the GL context current; the session is created on
 * that context. */
int xr_init(void *xdisplay, unsigned long xid, char *err, int errlen);
void xr_shutdown(void);

/* 1 = opaque, 3 = alpha blend (the mode the session was configured for). */
int xr_blend_mode(void);
/* GL internal format of the eye swapchains; render targets must use the same
 * one or the resolve blit is invalid. */
unsigned int xr_swapchain_format(void);
/* Current XrSessionState (1 = IDLE ... 5 = FOCUSED, 7 = EXITING). */
int xr_state(void);
void xr_size(int *w, int *h);

/* Drains runtime events. Returns 1 once the app must exit. */
int xr_poll(void);

/* Blocks for the next frame. Returns 1 when the app should render this frame,
 * 0 when the frame was already ended without layers (session not visible yet,
 * or the runtime asked to skip it). On 1, *out_time holds the predicted display
 * time in seconds. */
int xr_wait_frame(double *out_time);

/* Re-locates both eye views for the given display time. Returns 1 on success. */
int xr_locate_views(double display_time);

/* Row-major 3x4 device-to-local matrix of one eye view (already the eye pose,
 * i.e. what OpenVR reported as head * eyeToHead). */
void xr_eye_pose(int eye, float *out12);
/* fov in radians: left, right, up, down. */
void xr_eye_fov(int eye, float *out4);
/* Head (VIEW space) pose as a row-major 3x4 matrix. Returns 0 if unavailable. */
int xr_head_pose(double display_time, float *out12);

/* Acquires and waits for the swapchain image of one eye. Returns 1 and the GL
 * texture name on success. */
int xr_acquire(int eye, unsigned int *texture);

/* Releases both images and submits the projection layer. */
int xr_end_frame(double display_time);

/* Controller input. sync runs once per frame, after xr_wait_frame. hand is 0 for
 * left and 1 for right. */
int xr_sync_input(double display_time);
float xr_hand_trigger(int hand);
int xr_hand_trigger_click(int hand);
void xr_hand_stick(int hand, float *x, float *y);
/* Bitmask: 1=A, 2=B, 4=X, 8=Y, 16=stick click. */
int xr_hand_buttons(int hand);
/* aim 0 = grip pose, 1 = aim pose. Row-major 3x4 matrix. */
int xr_hand_pose(int hand, int aim, float *out12);
int xr_haptic(int hand, float amplitude);

// Interaction render model asset for one hand (0 = left, 1 = right). Returns a
// malloc'd glTF buffer the caller frees with free(), or null with err filled.
unsigned char *xr_model_gltf(int hand, int *out_len, char *err, int errlen);
/* Pose of one hand's render model space inside that hand's grip space. */
int xr_model_pose(int hand, double display_time, float *out12);


#ifdef __cplusplus
}
#endif

#endif
