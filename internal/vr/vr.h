#ifndef GOVR_VR_H
#define GOVR_VR_H

#ifdef __cplusplus
extern "C" {
#endif

/* Returns 1 when a headset is known to SteamVR. */
int govr_vr_present(void);

/* Initialises OpenVR in scene mode. Returns 0 on success, keeps a human
 * readable reason in err otherwise. */
int govr_vr_init(char *err, int errlen);

/* Same, but inputOnly brings SteamVR up as a background app: no compositor
 * frame loop, poses read straight from IVRSystem. Used when an OpenXR session
 * owns the display. */
int govr_vr_init_mode(int inputOnly, char *err, int errlen);
void govr_vr_shutdown(void);

/* Loads the SteamVR action manifest that binds app actions to buttons the
 * legacy button enum does not expose (the right-hand X). Returns 1 on success
 * and leaves a reason in err otherwise. */
int govr_vr_init_actions(const char *manifest_path, char *err, int errlen);
/* Reads the action set: stick/turn axes, trigger pull and the jump/dash
 * buttons. Returns 1 when actions are live, 0 when the app should fall back to
 * legacy controller state. */
int govr_vr_actions(float *move_x, float *move_y, float *turn_x, float *turn_y, float *trigger,
                    int *pause, int *model_next, int *model_prev, int *anim_next, int *anim_prev,
                    int *scrub_back, int *scrub_fwd);

void govr_vr_render_size(int *w, int *h);

/* Row-major matrices exactly as OpenVR returns them; the layout conversion to
 * OpenGL's column-major form happens in Go. */
void govr_vr_projection(int eye, float near_z, float far_z, float *out16);
void govr_vr_eye_to_head(int eye, float *out12);

/* Blocks until the compositor has new poses for this frame. */
int govr_vr_wait_poses(void);

/* Fills out12 with the row-major device-to-standing matrix. Returns 0 when the
 * pose is not valid this frame. */
int govr_vr_pose(int device, float *out12);
int govr_vr_model_name(unsigned int device, char *out, int outlen);
int govr_vr_device_info(unsigned int device, char *typ, int typlen, char *track, int tracklen,
                        char *serial, int seriallen, char *model, int modellen);
int govr_vr_model_load(unsigned int device, float *pos, float *nrm, unsigned short *idx,
                       int maxVerts, int maxIdx, int *nv, int *ni, char *name, int namelen);

int govr_vr_submit(int eye, unsigned int texture);

/* Returns 1 and fills event_type when an event was pending. */
int govr_vr_poll_event(int *event_type);

/* Aim pose of a controller (device -Z points where the user aims), row-major
 * 3x4. Returns 0 when the pose is not valid yet. */
int govr_vr_controller_aim(unsigned int device, float *out12);

/* hand: 0 = left, 1 = right. Returns 0 and fills device when present. */
int govr_vr_controller(int hand, unsigned int *device);
int govr_vr_controller_list(unsigned int *out, int max);
/* Raw stick axes, analog trigger and ulButtonPressed; the semantic button bits
 * are decoded in Go. */
int govr_vr_controller_touched(unsigned int device, unsigned long long *raw_touched);
int govr_vr_controller_state(unsigned int device, float *axis_x, float *axis_y, float *trigger_axis,
                             unsigned long long *raw_buttons);

/* Copies count raw rAxis x/y pairs into out2 (2 floats per axis, up to 5). */
int govr_vr_controller_axes(unsigned int device, float *out2, int count);

#ifdef __cplusplus
}
#endif

#endif
