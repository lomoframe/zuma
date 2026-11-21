#ifndef GOVR_GLSHIM_H
#define GOVR_GLSHIM_H

// GLEW includes <GL/glu.h> unless this is set. Nothing here calls GLU, and the
// header is missing from many cross sysroots.
#define GLEW_NO_GLU

#include <GL/glew.h>

#ifdef __cplusplus
extern "C" {
#endif

int govr_glew_init(void);
const char *govr_gl_version(void);

void govr_viewport(int x, int y, int w, int h);
void govr_clear_color(float r, float g, float b, float a);
void govr_clear(unsigned int mask);
void govr_enable(unsigned int cap);
void govr_disable(unsigned int cap);
void govr_depth_func(unsigned int f);
void govr_polygon_offset(float factor, float units);
void govr_cull_face(unsigned int m);
void govr_front_face(unsigned int m);
void govr_blend_func(unsigned int s, unsigned int d);
void govr_pixel_store_i(unsigned int pname, int v);

unsigned int govr_create_program(const char *vs, const char *fs, char *log, int loglen);
void govr_use_program(unsigned int p);
int govr_uniform_loc(unsigned int p, const char *name);
void govr_uniform_1i(int loc, int v);
void govr_uniform_1f(int loc, float v);
void govr_uniform_3f(int loc, float x, float y, float z);
void govr_uniform_4f(int loc, float x, float y, float z, float w);
void govr_uniform_mat3(int loc, const float *m);
void govr_uniform_mat4(int loc, const float *m);
void govr_uniform_mat4_array(int loc, int count, const float *m);

void govr_gen_vao(unsigned int *out);
void govr_bind_vao(unsigned int vao);
void govr_delete_vao(unsigned int vao);
void govr_gen_buffer(unsigned int *out);
void govr_delete_buffer(unsigned int buf);
void govr_bind_buffer(unsigned int target, unsigned int buf);
void govr_buffer_data(unsigned int target, const void *data, long size, unsigned int usage);
void govr_buffer_sub_data(unsigned int target, long offset, const void *data, long size);
void govr_bind_buffer_base(unsigned int target, unsigned int index, unsigned int buf);
void govr_enable_attrib(unsigned int loc);
void govr_attrib_ptr(unsigned int loc, int size, unsigned int type, unsigned char normalized, int stride, long offset);
void govr_attrib_int_ptr(unsigned int loc, int size, unsigned int type, int stride, long offset);
void govr_draw_elements(unsigned int mode, int count, unsigned int type);
void govr_draw_arrays(unsigned int mode, int first, int count);

void govr_gen_texture(unsigned int *out);
void govr_delete_texture(unsigned int tex);
void govr_bind_texture(unsigned int target, unsigned int tex);
void govr_tex_image_2d(unsigned int target, int level, int internalformat, int w, int h, int border,
                       unsigned int format, unsigned int type, const void *pixels);
void govr_tex_param_i(unsigned int target, unsigned int pname, int v);
void govr_active_texture(unsigned int unit);
void govr_generate_mipmap(unsigned int target);
void govr_uniform_block_binding(unsigned int p, const char *name, unsigned int binding);
void govr_delete_program(unsigned int p);

void govr_gen_fbo(unsigned int *out);
void govr_bind_fbo(unsigned int fb);
void govr_bind_fbo_target(unsigned int target, unsigned int fb);
void govr_fbo_texture_2d(unsigned int target, unsigned int attachment, unsigned int textarget, unsigned int tex);
void govr_gen_rbo(unsigned int *out);
void govr_bind_rbo(unsigned int rb);
void govr_rbo_storage(unsigned int target, unsigned int format, int w, int h);
void govr_rbo_storage_ms(unsigned int target, unsigned int format, int samples, int w, int h);
void govr_fbo_rbo(unsigned int target, unsigned int attachment, unsigned int rbtarget, unsigned int rb);
int govr_check_fbo(unsigned int target);
void govr_blit_color(int sw, int sh, int dw, int dh);

void govr_read_pixels(int x, int y, int w, int h, unsigned int format, unsigned int type, void *out);
void govr_depth_mask(unsigned char on);
void govr_flush(void);
unsigned int govr_get_error(void);

#ifdef __cplusplus
}
#endif

#endif
