#include "glshim.h"

#include <stdio.h>
#include <string.h>

int govr_glew_init(void) {
    glewExperimental = GL_TRUE;
    GLenum err = glewInit();
    if (err != GLEW_OK) {
        return (int)err;
    }
    /* glewInit on a core profile often leaves a spurious GL_INVALID_ENUM. */
    while (glGetError() != GL_NO_ERROR) {
    }
    return 0;
}

const char *govr_gl_version(void) {
    const GLubyte *v = glGetString(GL_VERSION);
    return v == NULL ? "" : (const char *)v;
}

void govr_viewport(int x, int y, int w, int h) { glViewport(x, y, w, h); }
void govr_clear_color(float r, float g, float b, float a) { glClearColor(r, g, b, a); }
void govr_clear(unsigned int mask) { glClear(mask); }
void govr_enable(unsigned int cap) { glEnable(cap); }
void govr_disable(unsigned int cap) { glDisable(cap); }
void govr_depth_func(unsigned int f) { glDepthFunc(f); }
void govr_polygon_offset(float factor, float units) { glPolygonOffset(factor, units); }
void govr_cull_face(unsigned int m) { glCullFace(m); }
void govr_front_face(unsigned int m) { glFrontFace(m); }
void govr_blend_func(unsigned int s, unsigned int d) { glBlendFunc(s, d); }
void govr_pixel_store_i(unsigned int pname, int v) { glPixelStorei(pname, v); }

static GLuint compile_stage(GLenum type, const char *src, char *log, int loglen) {
    GLuint sh = glCreateShader(type);
    glShaderSource(sh, 1, &src, NULL);
    glCompileShader(sh);
    GLint ok = 0;
    glGetShaderiv(sh, GL_COMPILE_STATUS, &ok);
    if (!ok) {
        glGetShaderInfoLog(sh, loglen, NULL, log);
        glDeleteShader(sh);
        return 0;
    }
    return sh;
}

unsigned int govr_create_program(const char *vs, const char *fs, char *log, int loglen) {
    log[0] = '\0';
    GLuint v = compile_stage(GL_VERTEX_SHADER, vs, log, loglen);
    if (!v) {
        return 0;
    }
    GLuint f = compile_stage(GL_FRAGMENT_SHADER, fs, log, loglen);
    if (!f) {
        glDeleteShader(v);
        return 0;
    }
    GLuint p = glCreateProgram();
    glAttachShader(p, v);
    glAttachShader(p, f);
    glLinkProgram(p);
    GLint ok = 0;
    glGetProgramiv(p, GL_LINK_STATUS, &ok);
    glDeleteShader(v);
    glDeleteShader(f);
    if (!ok) {
        glGetProgramInfoLog(p, loglen, NULL, log);
        glDeleteProgram(p);
        return 0;
    }
    return p;
}

void govr_use_program(unsigned int p) { glUseProgram(p); }

int govr_uniform_loc(unsigned int p, const char *name) { return glGetUniformLocation(p, name); }
void govr_uniform_1i(int loc, int v) { glUniform1i(loc, v); }
void govr_uniform_1f(int loc, float v) { glUniform1f(loc, v); }
void govr_uniform_3f(int loc, float x, float y, float z) { glUniform3f(loc, x, y, z); }
void govr_uniform_4f(int loc, float x, float y, float z, float w) { glUniform4f(loc, x, y, z, w); }
void govr_uniform_mat3(int loc, const float *m) { glUniformMatrix3fv(loc, 1, GL_FALSE, m); }
void govr_uniform_mat4(int loc, const float *m) { glUniformMatrix4fv(loc, 1, GL_FALSE, m); }
void govr_uniform_mat4_array(int loc, int count, const float *m) {
    glUniformMatrix4fv(loc, count, GL_FALSE, m);
}

void govr_gen_vao(unsigned int *out) { glGenVertexArrays(1, out); }
void govr_bind_vao(unsigned int vao) { glBindVertexArray(vao); }
void govr_delete_vao(unsigned int vao) { glDeleteVertexArrays(1, &vao); }
void govr_gen_buffer(unsigned int *out) { glGenBuffers(1, out); }
void govr_delete_buffer(unsigned int buf) { glDeleteBuffers(1, &buf); }
void govr_bind_buffer(unsigned int target, unsigned int buf) { glBindBuffer(target, buf); }
void govr_buffer_data(unsigned int target, const void *data, long size, unsigned int usage) {
    glBufferData(target, (GLsizeiptr)size, data, usage);
}
void govr_buffer_sub_data(unsigned int target, long offset, const void *data, long size) {
    glBufferSubData(target, (GLintptr)offset, (GLsizeiptr)size, data);
}
void govr_bind_buffer_base(unsigned int target, unsigned int index, unsigned int buf) {
    glBindBufferBase(target, index, buf);
}
void govr_enable_attrib(unsigned int loc) { glEnableVertexAttribArray(loc); }
void govr_attrib_ptr(unsigned int loc, int size, unsigned int type, unsigned char normalized,
                     int stride, long offset) {
    glVertexAttribPointer(loc, size, type, normalized, stride, (const void *)offset);
}
void govr_attrib_int_ptr(unsigned int loc, int size, unsigned int type, int stride, long offset) {
    glVertexAttribIPointer(loc, size, type, stride, (const void *)offset);
}
void govr_draw_elements(unsigned int mode, int count, unsigned int type) {
    glDrawElements(mode, count, type, NULL);
}
void govr_draw_arrays(unsigned int mode, int first, int count) { glDrawArrays(mode, first, count); }

void govr_gen_texture(unsigned int *out) { glGenTextures(1, out); }
void govr_delete_texture(unsigned int tex) { glDeleteTextures(1, &tex); }
void govr_bind_texture(unsigned int target, unsigned int tex) { glBindTexture(target, tex); }
void govr_tex_image_2d(unsigned int target, int level, int internalformat, int w, int h, int border,
                       unsigned int format, unsigned int type, const void *pixels) {
    glTexImage2D(target, level, internalformat, w, h, border, format, type, pixels);
}
void govr_tex_param_i(unsigned int target, unsigned int pname, int v) { glTexParameteri(target, pname, v); }
void govr_active_texture(unsigned int unit) { glActiveTexture(unit); }
void govr_generate_mipmap(unsigned int target) { glGenerateMipmap(target); }
void govr_delete_program(unsigned int p) { glDeleteProgram(p); }
void govr_uniform_block_binding(unsigned int p, const char *name, unsigned int binding) {
    glUniformBlockBinding(p, glGetUniformBlockIndex(p, name), binding);
}

void govr_gen_fbo(unsigned int *out) { glGenFramebuffers(1, out); }
void govr_bind_fbo(unsigned int fb) { glBindFramebuffer(GL_FRAMEBUFFER, fb); }
void govr_bind_fbo_target(unsigned int target, unsigned int fb) { glBindFramebuffer(target, fb); }
void govr_fbo_texture_2d(unsigned int target, unsigned int attachment, unsigned int textarget,
                         unsigned int tex) {
    glFramebufferTexture2D(target, attachment, textarget, tex, 0);
}
void govr_gen_rbo(unsigned int *out) { glGenRenderbuffers(1, out); }
void govr_bind_rbo(unsigned int rb) { glBindRenderbuffer(GL_RENDERBUFFER, rb); }
void govr_rbo_storage(unsigned int target, unsigned int format, int w, int h) {
    glRenderbufferStorage(target, format, w, h);
}
void govr_rbo_storage_ms(unsigned int target, unsigned int format, int samples, int w, int h) {
    glRenderbufferStorageMultisample(target, samples, format, w, h);
}
void govr_fbo_rbo(unsigned int target, unsigned int attachment, unsigned int rbtarget, unsigned int rb) {
    glFramebufferRenderbuffer(target, attachment, rbtarget, rb);
}
int govr_check_fbo(unsigned int target) { return (int)glCheckFramebufferStatus(target); }

// govr_blit_color resolves the whole read buffer into the draw buffer; the
// caller binds GL_READ_FRAMEBUFFER / GL_DRAW_FRAMEBUFFER beforehand.
void govr_blit_color(int sw, int sh, int dw, int dh) {
    glBlitFramebuffer(0, 0, sw, sh, 0, 0, dw, dh, GL_COLOR_BUFFER_BIT, GL_NEAREST);
}

void govr_read_pixels(int x, int y, int w, int h, unsigned int format, unsigned int type, void *out) {
    glReadPixels(x, y, w, h, format, type, out);
}
void govr_depth_mask(unsigned char on) { glDepthMask(on ? GL_TRUE : GL_FALSE); }
void govr_flush(void) { glFlush(); }
unsigned int govr_get_error(void) { return (unsigned int)glGetError(); }

