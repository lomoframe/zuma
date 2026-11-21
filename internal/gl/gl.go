// Package gl is a thin cgo wrapper around the handful of OpenGL 4.1 core
// entry points govr needs. It exists because no GL binding module was
// fetchable in this environment; keeping the surface small keeps cgo cost
// negligible (well under a hundred calls per frame).
package gl

/*
#cgo LDFLAGS: -lGLEW -lGL
#include <stdlib.h>
#include "glshim.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	DepthTest      = uint32(C.GL_DEPTH_TEST)
	CullFace       = uint32(C.GL_CULL_FACE)
	ColorBufferBit = uint32(C.GL_COLOR_BUFFER_BIT)
	DepthBufferBit = uint32(C.GL_DEPTH_BUFFER_BIT)

	Triangles     = uint32(C.GL_TRIANGLES)
	UnsignedInt   = uint32(C.GL_UNSIGNED_INT)
	Float         = uint32(C.GL_FLOAT)
	UnsignedByte  = uint32(C.GL_UNSIGNED_BYTE)
	UnsignedShort = uint32(C.GL_UNSIGNED_SHORT)

	ArrayBuffer        = uint32(C.GL_ARRAY_BUFFER)
	ElementArrayBuffer = uint32(C.GL_ELEMENT_ARRAY_BUFFER)
	UniformBuffer      = uint32(C.GL_UNIFORM_BUFFER)
	StaticDraw         = uint32(C.GL_STATIC_DRAW)

	Texture2D        = uint32(C.GL_TEXTURE_2D)
	RGBA             = uint32(C.GL_RGBA)
	RGBA8            = uint32(C.GL_RGBA8)
	SRGB8Alpha8      = uint32(C.GL_SRGB8_ALPHA8)
	Linear           = uint32(C.GL_LINEAR)
	Nearest          = uint32(C.GL_NEAREST)
	LinearMipmap     = uint32(C.GL_LINEAR_MIPMAP_LINEAR)
	LinearMipmapNear = uint32(0x2701) // GL_LINEAR_MIPMAP_NEAREST
	NearMipmapNear   = uint32(0x2700) // GL_NEAREST_MIPMAP_NEAREST
	NearMipmapLinear = uint32(0x2702) // GL_NEAREST_MIPMAP_LINEAR
	ClampToEdge      = uint32(C.GL_CLAMP_TO_EDGE)
	Repeat           = uint32(C.GL_REPEAT)
	MirroredRepeat   = uint32(C.GL_MIRRORED_REPEAT)
	Texture0         = uint32(C.GL_TEXTURE0)
	Texture1         = uint32(0x84C1) // GL_TEXTURE1
	TextureMinFilter = uint32(C.GL_TEXTURE_MIN_FILTER)
	TextureMagFilter = uint32(C.GL_TEXTURE_MAG_FILTER)
	TextureWrapS     = uint32(C.GL_TEXTURE_WRAP_S)
	TextureWrapT     = uint32(C.GL_TEXTURE_WRAP_T)

	// Blend state and the dynamic-draw hint are passed straight through to GL.
	Blend            = uint32(0x0BE2) // GL_BLEND
	One              = uint32(0x0001) // GL_ONE
	SrcAlpha         = uint32(0x0302) // GL_SRC_ALPHA
	OneMinusSrcAlpha = uint32(0x0303) // GL_ONE_MINUS_SRC_ALPHA
	DynamicDraw      = uint32(0x88E8) // GL_DYNAMIC_DRAW

	Framebuffer      = uint32(C.GL_FRAMEBUFFER)
	ReadFramebuffer  = uint32(0x8CA8) // GL_READ_FRAMEBUFFER
	DrawFramebuffer  = uint32(0x8CA9) // GL_DRAW_FRAMEBUFFER
	Renderbuffer     = uint32(C.GL_RENDERBUFFER)
	ColorAttachment0 = uint32(C.GL_COLOR_ATTACHMENT0)
	DepthAttachment  = uint32(C.GL_DEPTH_ATTACHMENT)
	DepthComponent24 = uint32(C.GL_DEPTH_COMPONENT24)
	FramebufferOK    = uint32(C.GL_FRAMEBUFFER_COMPLETE)
	Lequal           = uint32(C.GL_LEQUAL)
	Less             = uint32(C.GL_LESS)
	Back             = uint32(C.GL_BACK)
	CCW              = uint32(C.GL_CCW)
	UnpackAlignment  = uint32(C.GL_UNPACK_ALIGNMENT)
	PackAlignment    = uint32(C.GL_PACK_ALIGNMENT)
)

// Init loads the GL function table. The calling goroutine must have a current
// GL context.
func Init() error {
	if code := C.govr_glew_init(); code != 0 {
		return fmt.Errorf("glewInit failed (code %d)", int(code))
	}
	return nil
}

func Viewport(x, y, w, h int32) { C.govr_viewport(C.int(x), C.int(y), C.int(w), C.int(h)) }

// Version reports the driver's GL_VERSION string.
func Version() string { return C.GoString(C.govr_gl_version()) }
func ClearColor(r, g, b, a float32) {
	C.govr_clear_color(C.float(r), C.float(g), C.float(b), C.float(a))
}
func Clear(mask uint32)                { C.govr_clear(C.uint(mask)) }
func Enable(cap uint32)                { C.govr_enable(C.uint(cap)) }
func Disable(cap uint32)               { C.govr_disable(C.uint(cap)) }
func DepthFunc(f uint32)               { C.govr_depth_func(C.uint(f)) }
func CullFaceMode(m uint32)            { C.govr_cull_face(C.uint(m)) }
func FrontFace(m uint32)               { C.govr_front_face(C.uint(m)) }
func BlendFunc(src, dst uint32)        { C.govr_blend_func(C.uint(src), C.uint(dst)) }
func PixelStore(pname uint32, v int32) { C.govr_pixel_store_i(C.uint(pname), C.int(v)) }
func UseProgram(p uint32)              { C.govr_use_program(C.uint(p)) }
func Flush()                           { C.govr_flush() }

// DepthMask controls whether depth writes are enabled.
func DepthMask(on bool) {
	if on {
		C.govr_depth_mask(1)
	} else {
		C.govr_depth_mask(0)
	}
}
func Error() uint32 { return uint32(C.govr_get_error()) }

type Program uint32

// NewProgram compiles and links a vertex/fragment shader pair.
func NewProgram(vertexSrc, fragmentSrc string) (Program, error) {
	cvs := C.CString(vertexSrc)
	cfs := C.CString(fragmentSrc)
	defer C.free(unsafe.Pointer(cvs))
	defer C.free(unsafe.Pointer(cfs))

	buf := make([]byte, 4096)
	id := C.govr_create_program(cvs, cfs, (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	if id == 0 {
		return 0, fmt.Errorf("shader build failed: %s", cString(buf))
	}
	return Program(id), nil
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func (p Program) Bind()   { UseProgram(uint32(p)) }
func (p Program) Delete() { C.govr_delete_program(C.uint(p)) }

func (p Program) Loc(name string) int32 {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	return int32(C.govr_uniform_loc(C.uint(p), cn))
}

func (p Program) Int(loc int32, v int32)     { C.govr_uniform_1i(C.int(loc), C.int(v)) }
func (p Program) Float(loc int32, v float32) { C.govr_uniform_1f(C.int(loc), C.float(v)) }
func (p Program) Vec3(loc int32, x, y, z float32) {
	C.govr_uniform_3f(C.int(loc), C.float(x), C.float(y), C.float(z))
}
func (p Program) Vec4(loc int32, x, y, z, w float32) {
	C.govr_uniform_4f(C.int(loc), C.float(x), C.float(y), C.float(z), C.float(w))
}

// Mat3 uploads a 3x3 matrix, used for per-ball texture orientation.
func (p Program) Mat3(loc int32, m [9]float32) {
	C.govr_uniform_mat3(C.int(loc), (*C.float)(unsafe.Pointer(&m[0])))
}

func (p Program) Mat4(loc int32, m [16]float32) {
	C.govr_uniform_mat4(C.int(loc), (*C.float)(unsafe.Pointer(&m[0])))
}

// Mat4Array uploads a matrix array, e.g. a joint palette.
func (p Program) Mat4Array(loc int32, mats []float32) {
	if len(mats) == 0 {
		return
	}
	C.govr_uniform_mat4_array(C.int(loc), C.int(len(mats)/16), (*C.float)(unsafe.Pointer(&mats[0])))
}
func GenVAO() uint32 {
	var v C.uint
	C.govr_gen_vao(&v)
	return uint32(v)
}
func BindVAO(v uint32)   { C.govr_bind_vao(C.uint(v)) }
func DeleteVAO(v uint32) { C.govr_delete_vao(C.uint(v)) }

func GenBuffer() uint32 {
	var b C.uint
	C.govr_gen_buffer(&b)
	return uint32(b)
}
func BindBuffer(target, buf uint32) { C.govr_bind_buffer(C.uint(target), C.uint(buf)) }
func DeleteBuffer(buf uint32)       { C.govr_delete_buffer(C.uint(buf)) }
func BufferData(target uint32, data []byte, usage uint32) {
	var ptr unsafe.Pointer
	if len(data) > 0 {
		ptr = unsafe.Pointer(&data[0])
	}
	C.govr_buffer_data(C.uint(target), ptr, C.long(len(data)), C.uint(usage))
}

// BufferSubData replaces part of the bound buffer without reallocating it.
func BufferSubData(target uint32, offset int, data []byte) {
	var ptr unsafe.Pointer
	if len(data) > 0 {
		ptr = unsafe.Pointer(&data[0])
	}
	C.govr_buffer_sub_data(C.uint(target), C.long(offset), ptr, C.long(len(data)))
}

// BindBufferBase binds buf to the uniform/texture buffer binding point index.
func BindBufferBase(target, index, buf uint32) {
	C.govr_bind_buffer_base(C.uint(target), C.uint(index), C.uint(buf))
}
func EnableAttrib(loc uint32) { C.govr_enable_attrib(C.uint(loc)) }
func AttribIntPtr(loc uint32, size int32, typ uint32, stride int32, offset int) {
	C.govr_attrib_int_ptr(C.uint(loc), C.int(size), C.uint(typ), C.int(stride), C.long(offset))
}

func AttribPtr(loc uint32, size int32, typ uint32, normalized bool, stride int32, offset int) {
	norm := C.uchar(0)
	if normalized {
		norm = 1
	}
	C.govr_attrib_ptr(C.uint(loc), C.int(size), C.uint(typ), norm, C.int(stride), C.long(offset))
}
func DrawElements(mode uint32, count int32, typ uint32) {
	C.govr_draw_elements(C.uint(mode), C.int(count), C.uint(typ))
}
func DrawArrays(mode uint32, first, count int32) {
	C.govr_draw_arrays(C.uint(mode), C.int(first), C.int(count))
}

func GenTexture() uint32 {
	var t C.uint
	C.govr_gen_texture(&t)
	return uint32(t)
}
func BindTexture(target, tex uint32) { C.govr_bind_texture(C.uint(target), C.uint(tex)) }
func DeleteTexture(tex uint32)       { C.govr_delete_texture(C.uint(tex)) }
func TexImage2D(target uint32, internalFormat int32, w, h int32, format, typ uint32, pixels unsafe.Pointer) {
	C.govr_tex_image_2d(C.uint(target), 0, C.int(internalFormat), C.int(w), C.int(h), 0,
		C.uint(format), C.uint(typ), pixels)
}
func TexParamI(target, pname uint32, v int32) {
	C.govr_tex_param_i(C.uint(target), C.uint(pname), C.int(v))
}
func ActiveTexture(unit uint32) { C.govr_active_texture(C.uint(unit)) }

// GenerateMipmap builds the mip chain of the bound texture.
func GenerateMipmap(target uint32) { C.govr_generate_mipmap(C.uint(target)) }

// UniformBlockBinding points the program's named uniform block at binding.
func (p Program) UniformBlockBinding(name string, binding uint32) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	C.govr_uniform_block_binding(C.uint(p), cn, C.uint(binding))
}

// RenderTarget is an offscreen framebuffer used for eye textures. With
// samples > 1 the scene is drawn into a multisampled framebuffer and Resolve
// blits it into the single-sample texture that goes to the compositor.
type RenderTarget struct {
	FBO, Tex, RBO uint32 // resolved texture, its framebuffer and depth
	W, H, Samples int32
	draw          uint32 // multisampled draw target; FBO when Samples <= 1
}

// NewRenderTarget builds a colour+depth target. samples is the MSAA sample
// count; 0 or 1 means no multisampling.
func NewRenderTarget(w, h, samples int32) (*RenderTarget, error) {
	return NewRenderTargetFormat(w, h, samples, RGBA8)
}

// NewRenderTargetFormat is NewRenderTarget with an explicit colour format. The
// OpenXR path uses it so the resolve blit writes into a texture with exactly
// the swapchain's format, which glBlitFramebuffer requires.
func NewRenderTargetFormat(w, h, samples int32, format uint32) (*RenderTarget, error) {
	if samples < 0 {
		samples = 0
	}
	rt := &RenderTarget{W: w, H: h, Samples: samples}
	rt.FBO = GenFramebuffer()
	rt.Tex = GenTexture()
	rt.RBO = GenRenderbuffer()

	BindTexture(Texture2D, rt.Tex)
	TexImage2D(Texture2D, int32(format), w, h, RGBA, UnsignedByte, nil)
	TexParamI(Texture2D, TextureMinFilter, int32(Linear))
	TexParamI(Texture2D, TextureMagFilter, int32(Linear))
	TexParamI(Texture2D, TextureWrapS, int32(ClampToEdge))
	TexParamI(Texture2D, TextureWrapT, int32(ClampToEdge))
	BindTexture(Texture2D, 0)

	BindRenderbuffer(rt.RBO)
	RenderbufferStorage(w, h)
	BindRenderbuffer(0)

	BindFramebuffer(rt.FBO)
	C.govr_fbo_texture_2d(C.uint(Framebuffer), C.uint(ColorAttachment0), C.uint(Texture2D), C.uint(rt.Tex))
	C.govr_fbo_rbo(C.uint(Framebuffer), C.uint(DepthAttachment), C.uint(Renderbuffer), C.uint(rt.RBO))
	status := uint32(C.govr_check_fbo(C.uint(Framebuffer)))
	BindFramebuffer(0)
	if status != FramebufferOK {
		return nil, fmt.Errorf("framebuffer incomplete (status 0x%x)", status)
	}

	rt.draw = rt.FBO
	if samples > 1 {
		rt.draw = GenFramebuffer()
		color, depth := GenRenderbuffer(), GenRenderbuffer()
		BindRenderbuffer(color)
		RenderbufferStorageMS(format, w, h, samples)
		BindRenderbuffer(depth)
		RenderbufferStorageMS(DepthComponent24, w, h, samples)
		BindRenderbuffer(0)

		BindFramebuffer(rt.draw)
		C.govr_fbo_rbo(C.uint(Framebuffer), C.uint(ColorAttachment0), C.uint(Renderbuffer), C.uint(color))
		C.govr_fbo_rbo(C.uint(Framebuffer), C.uint(DepthAttachment), C.uint(Renderbuffer), C.uint(depth))
		status = uint32(C.govr_check_fbo(C.uint(Framebuffer)))
		BindFramebuffer(0)
		if status != FramebufferOK {
			return nil, fmt.Errorf("multisample framebuffer incomplete (status 0x%x)", status)
		}
	}
	return rt, nil
}

// Bind makes the draw framebuffer current: the multisampled one when MSAA is
// on, otherwise the texture-backed one.
func (rt *RenderTarget) Bind() { BindFramebuffer(rt.draw) }

// Resolve copies the multisampled buffer into the submitted texture. It is a
// no-op without MSAA.
func (rt *RenderTarget) Resolve() {
	if rt.Samples <= 1 {
		return
	}
	BindFramebufferTarget(ReadFramebuffer, rt.draw)
	BindFramebufferTarget(DrawFramebuffer, rt.FBO)
	BlitColor(rt.W, rt.H, rt.W, rt.H)
	BindFramebuffer(0)
}

func (rt *RenderTarget) Unbind() { BindFramebuffer(0) }

// SetColorTexture points the single-sample framebuffer at an external colour
// texture, such as an OpenXR swapchain image. Resolve blits the multisampled
// frame into it, so one target can feed whichever image the runtime hands out.
// The texture keeps the target's size; the caller must pass one of that size.
func (rt *RenderTarget) SetColorTexture(tex uint32) {
	BindFramebuffer(rt.FBO)
	C.govr_fbo_texture_2d(C.uint(Framebuffer), C.uint(ColorAttachment0), C.uint(Texture2D), C.uint(tex))
	BindFramebuffer(0)
	rt.Tex = tex
}

func GenFramebuffer() uint32 {
	var f C.uint
	C.govr_gen_fbo(&f)
	return uint32(f)
}
func BindFramebuffer(fb uint32) { C.govr_bind_fbo(C.uint(fb)) }

// BindFramebufferTarget binds GL_READ_FRAMEBUFFER / GL_DRAW_FRAMEBUFFER.
func BindFramebufferTarget(target, fb uint32) {
	C.govr_bind_fbo_target(C.uint(target), C.uint(fb))
}

// BlitColor resolves the bound read buffer into the bound draw buffer.
func BlitColor(sw, sh, dw, dh int32) { C.govr_blit_color(C.int(sw), C.int(sh), C.int(dw), C.int(dh)) }

// CheckFramebuffer reports the completeness of the currently bound framebuffer.
func CheckFramebuffer() uint32 { return uint32(C.govr_check_fbo(C.uint(Framebuffer))) }
func GenRenderbuffer() uint32 {
	var r C.uint
	C.govr_gen_rbo(&r)
	return uint32(r)
}
func BindRenderbuffer(rb uint32) { C.govr_bind_rbo(C.uint(rb)) }
func RenderbufferStorage(w, h int32) {
	C.govr_rbo_storage(C.uint(Renderbuffer), C.uint(DepthComponent24), C.int(w), C.int(h))
}

// RenderbufferStorageMS allocates multisampled storage for the bound buffer.
func RenderbufferStorageMS(format uint32, w, h, samples int32) {
	C.govr_rbo_storage_ms(C.uint(Renderbuffer), C.uint(format), C.int(samples), C.int(w), C.int(h))
}

// ReadPixels copies the bound framebuffer into a tightly packed RGBA buffer.
func ReadPixels(x, y, w, h int32, out []byte) {
	C.govr_read_pixels(C.int(x), C.int(y), C.int(w), C.int(h), C.uint(RGBA),
		C.uint(UnsignedByte), unsafe.Pointer(&out[0]))
}
