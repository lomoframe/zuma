// Package gltf reads just enough of a binary glTF (.glb) container to report
// its structure and to pull vertex, index and texture bytes out of it. It is
// deliberately narrow: no animations, no skinning, no extension parsing.
package gltf

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

const (
	magicGLTF = 0x46546C67 // "glTF"
	chunkJSON = 0x4E4F534A // "JSON"
	chunkBIN  = 0x004E4942 // "BIN\0"
)

// Document is a parsed .glb: the JSON chunk plus the binary chunk it indexes.
type Document struct {
	Asset struct {
		Generator string `json:"generator"`
		Version   string `json:"version"`
	} `json:"asset"`
	Nodes       []Node       `json:"nodes"`
	Meshes      []Mesh       `json:"meshes"`
	Accessors   []Accessor   `json:"accessors"`
	BufferViews []BufferView `json:"bufferViews"`
	Buffers     []Buffer     `json:"buffers"`
	Materials   []Material   `json:"materials"`
	Textures    []Texture    `json:"textures"`
	Images      []Image      `json:"images"`

	BIN []byte `json:"-"`
}

type Node struct {
	Name     string `json:"name"`
	Mesh     *int   `json:"mesh"`
	Children []int  `json:"children"`
	Skin     *int   `json:"skin"`
}

type Mesh struct {
	Name       string      `json:"name"`
	Primitives []Primitive `json:"primitives"`
}

type Primitive struct {
	Attributes map[string]int `json:"attributes"`
	Indices    *int           `json:"indices"`
	Material   *int           `json:"material"`
	Mode       *int           `json:"mode"`
}

type Accessor struct {
	BufferView    *int   `json:"bufferView"`
	ByteOffset    int    `json:"byteOffset"`
	ComponentType int    `json:"componentType"`
	Normalized    bool   `json:"normalized"`
	Count         int    `json:"count"`
	Type          string `json:"type"`
}

type BufferView struct {
	Buffer     int `json:"buffer"`
	ByteOffset int `json:"byteOffset"`
	ByteLength int `json:"byteLength"`
	ByteStride int `json:"byteStride"`
}

type Buffer struct {
	ByteLength int    `json:"byteLength"`
	URI        string `json:"uri"`
}

type Material struct {
	Name string `json:"name"`
	PBR  struct {
		BaseColorTexture *struct {
			Index int `json:"index"`
		} `json:"baseColorTexture"`
	} `json:"pbrMetallicRoughness"`
}

type Texture struct {
	Source *int `json:"source"`
}

type Image struct {
	Name       string `json:"name"`
	MimeType   string `json:"mimeType"`
	BufferView *int   `json:"bufferView"`
	URI        string `json:"uri"`
}

// ParseGLB splits a .glb into its JSON document and binary chunk.
func ParseGLB(data []byte) (*Document, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("glb: %d bytes, too short", len(data))
	}
	if binary.LittleEndian.Uint32(data) != magicGLTF {
		return nil, fmt.Errorf("glb: bad magic %08x", binary.LittleEndian.Uint32(data))
	}
	length := int(binary.LittleEndian.Uint32(data[8:]))
	if length > len(data) {
		return nil, fmt.Errorf("glb: header says %d bytes, have %d", length, len(data))
	}
	doc := &Document{}
	seenJSON := false
	for off := 12; off+8 <= length; {
		clen := int(binary.LittleEndian.Uint32(data[off:]))
		ctype := binary.LittleEndian.Uint32(data[off+4:])
		off += 8
		if clen < 0 || off+clen > length {
			return nil, fmt.Errorf("glb: chunk overruns file at %d", off)
		}
		chunk := data[off : off+clen]
		switch ctype {
		case chunkJSON:
			if err := json.Unmarshal(bytes.TrimRight(chunk, " \x00"), doc); err != nil {
				return nil, err
			}
			seenJSON = true
		case chunkBIN:
			doc.BIN = chunk
		}
		off += clen + (4-clen%4)%4
	}
	if !seenJSON {
		return nil, fmt.Errorf("glb: no JSON chunk")
	}
	if len(doc.Buffers) > 0 && doc.Buffers[0].URI == "" && len(doc.BIN) < doc.Buffers[0].ByteLength {
		return nil, fmt.Errorf("glb: bin chunk %d < buffer %d", len(doc.BIN), doc.Buffers[0].ByteLength)
	}
	return doc, nil
}

// Verts counts the vertices of every POSITION attribute.
func (d *Document) Verts() int {
	n := 0
	for _, m := range d.Meshes {
		for _, p := range m.Primitives {
			if acc, ok := p.Attributes["POSITION"]; ok && acc < len(d.Accessors) {
				n += d.Accessors[acc].Count
			}
		}
	}
	return n
}

// Tris counts triangles, treating every primitive as TRIANGLES (mode 4).
func (d *Document) Tris() int {
	n := 0
	for _, m := range d.Meshes {
		for _, p := range m.Primitives {
			if p.Mode != nil && *p.Mode != 4 {
				continue
			}
			if p.Indices != nil && *p.Indices < len(d.Accessors) {
				n += d.Accessors[*p.Indices].Count / 3
			} else if acc, ok := p.Attributes["POSITION"]; ok && acc < len(d.Accessors) {
				n += d.Accessors[acc].Count / 3
			}
		}
	}
	return n
}

// NodeNames lists mesh-bearing nodes in document order.
func (d *Document) NodeNames() []string {
	out := make([]string, 0, len(d.Nodes))
	for _, n := range d.Nodes {
		if n.Mesh != nil {
			out = append(out, n.Name)
		}
	}
	return out
}

// Floats decodes an accessor of float or integer components into []float32 and
// reports how many components each element has.
func (d *Document) Floats(index int) ([]float32, int, error) {
	if index < 0 || index >= len(d.Accessors) {
		return nil, 0, fmt.Errorf("glb: accessor %d out of range", index)
	}
	acc := d.Accessors[index]
	comps, err := componentCount(acc.Type)
	if err != nil {
		return nil, 0, err
	}
	raw, stride, err := d.view(acc)
	if err != nil {
		return nil, 0, err
	}
	out := make([]float32, acc.Count*comps)
	for i := 0; i < acc.Count; i++ {
		base := i * stride
		for c := 0; c < comps; c++ {
			p := base + c*componentSize(acc.ComponentType)
			if p+componentSize(acc.ComponentType) > len(raw) {
				return nil, 0, fmt.Errorf("glb: accessor %d overruns buffer", index)
			}
			out[i*comps+c] = readComponent(raw[p:], acc.ComponentType, acc.Normalized)
		}
	}
	return out, comps, nil
}

// Uints decodes an index accessor.
func (d *Document) Uints(index int) ([]uint32, error) {
	if index < 0 || index >= len(d.Accessors) {
		return nil, fmt.Errorf("glb: accessor %d out of range", index)
	}
	acc := d.Accessors[index]
	raw, stride, err := d.view(acc)
	if err != nil {
		return nil, err
	}
	size := componentSize(acc.ComponentType)
	out := make([]uint32, acc.Count)
	for i := 0; i < acc.Count; i++ {
		p := i * stride
		if p+size > len(raw) {
			return nil, fmt.Errorf("glb: accessor %d overruns buffer", index)
		}
		switch size {
		case 1:
			out[i] = uint32(raw[p])
		case 2:
			out[i] = uint32(binary.LittleEndian.Uint16(raw[p:]))
		case 4:
			out[i] = binary.LittleEndian.Uint32(raw[p:])
		}
	}
	return out, nil
}

// ImageBytes returns the encoded bytes of one image (PNG or JPEG as stored).
func (d *Document) ImageBytes(index int) ([]byte, error) {
	if index < 0 || index >= len(d.Images) {
		return nil, fmt.Errorf("glb: image %d out of range", index)
	}
	img := d.Images[index]
	if img.BufferView == nil {
		return nil, fmt.Errorf("glb: image %d has no bufferView (uri %q)", index, img.URI)
	}
	if *img.BufferView >= len(d.BufferViews) {
		return nil, fmt.Errorf("glb: image %d bufferView out of range", index)
	}
	bv := d.BufferViews[*img.BufferView]
	if bv.ByteOffset+bv.ByteLength > len(d.BIN) {
		return nil, fmt.Errorf("glb: image %d overruns bin chunk", index)
	}
	return d.BIN[bv.ByteOffset : bv.ByteOffset+bv.ByteLength], nil
}

func (d *Document) view(acc Accessor) ([]byte, int, error) {
	if acc.BufferView == nil {
		return nil, 0, fmt.Errorf("glb: accessor without bufferView")
	}
	if *acc.BufferView >= len(d.BufferViews) {
		return nil, 0, fmt.Errorf("glb: bufferView %d out of range", *acc.BufferView)
	}
	bv := d.BufferViews[*acc.BufferView]
	if bv.Buffer != 0 {
		return nil, 0, fmt.Errorf("glb: buffer %d unsupported (only the glb bin chunk)", bv.Buffer)
	}
	comps, err := componentCount(acc.Type)
	if err != nil {
		return nil, 0, err
	}
	elem := comps * componentSize(acc.ComponentType)
	stride := bv.ByteStride
	if stride == 0 {
		stride = elem
	}
	if acc.Count <= 0 {
		return nil, 0, fmt.Errorf("glb: accessor with no elements")
	}
	// The accessor's own offset shifts the first element inside the view, so the
	// window is offset + last element only: checking the whole view length here
	// rejects valid interleaved accessors such as the trigger's normals.
	start := bv.ByteOffset + acc.ByteOffset
	need := stride*(acc.Count-1) + elem
	if start < 0 || start+need > len(d.BIN) {
		return nil, 0, fmt.Errorf("glb: accessor window overruns bin chunk")
	}
	return d.BIN[start : start+need], stride, nil
}

func componentCount(typ string) (int, error) {
	switch typ {
	case "SCALAR":
		return 1, nil
	case "VEC2":
		return 2, nil
	case "VEC3":
		return 3, nil
	case "VEC4":
		return 4, nil
	case "MAT4":
		return 16, nil
	}
	return 0, fmt.Errorf("glb: accessor type %q unsupported", typ)
}

func componentSize(ct int) int {
	switch ct {
	case 5120, 5121: // int8, uint8
		return 1
	case 5122, 5123: // int16, uint16
		return 2
	case 5125, 5126: // uint32, float32
		return 4
	}
	return 4
}

func readComponent(b []byte, ct int, normalized bool) float32 {
	switch ct {
	case 5120: // int8
		v := float32(int8(b[0]))
		if normalized {
			if v < -127 {
				v = -127
			}
			return v / 127
		}
		return v
	case 5121: // uint8
		v := float32(b[0])
		if normalized {
			return v / 255
		}
		return v
	case 5122: // int16
		v := float32(int16(binary.LittleEndian.Uint16(b)))
		if normalized {
			if v < -32767 {
				v = -32767
			}
			return v / 32767
		}
		return v
	case 5123: // uint16
		v := float32(binary.LittleEndian.Uint16(b))
		if normalized {
			return v / 65535
		}
		return v
	case 5125: // uint32
		return float32(binary.LittleEndian.Uint32(b))
	case 5126: // float32
		return math.Float32frombits(binary.LittleEndian.Uint32(b))
	}
	return 0
}

// MeshByNode flattens the primitives of the mesh attached to the named node
// into POSITION/NORMAL/index arrays. These controller assets carry no node
// transforms, so the vertices are already in the model's local space.
func (d *Document) MeshByNode(name string) (pos, nrm []float32, idx []uint32, err error) {
	node := -1
	for i := range d.Nodes {
		if d.Nodes[i].Name == name && d.Nodes[i].Mesh != nil {
			node = i
			break
		}
	}
	if node < 0 {
		return nil, nil, nil, fmt.Errorf("glb: no node %q with a mesh", name)
	}
	meshIdx := *d.Nodes[node].Mesh
	if meshIdx < 0 || meshIdx >= len(d.Meshes) {
		return nil, nil, nil, fmt.Errorf("glb: node %q mesh %d out of range", name, meshIdx)
	}
	for _, prim := range d.Meshes[meshIdx].Primitives {
		posAcc, ok := prim.Attributes["POSITION"]
		if !ok {
			continue
		}
		p, comps, err := d.Floats(posAcc)
		if err != nil {
			return nil, nil, nil, err
		}
		if comps != 3 {
			return nil, nil, nil, fmt.Errorf("glb: POSITION has %d components", comps)
		}
		base := uint32(len(pos) / 3)
		pos = append(pos, p...)
		if nAcc, ok := prim.Attributes["NORMAL"]; ok {
			n, ncomps, err := d.Floats(nAcc)
			if err != nil {
				return nil, nil, nil, err
			}
			if ncomps != 3 {
				return nil, nil, nil, fmt.Errorf("glb: NORMAL has %d components", ncomps)
			}
			nrm = append(nrm, n...)
		} else {
			// Flat fallback so the mesh still shades: +Y per vertex.
			for i := 0; i < len(p)/3; i++ {
				nrm = append(nrm, 0, 1, 0)
			}
		}
		if prim.Indices != nil {
			ii, err := d.Uints(*prim.Indices)
			if err != nil {
				return nil, nil, nil, err
			}
			for _, v := range ii {
				idx = append(idx, v+base)
			}
		} else {
			for i := uint32(0); i < uint32(len(p)/3); i++ {
				idx = append(idx, base+i)
			}
		}
	}
	if len(pos) == 0 || len(idx) == 0 {
		return nil, nil, nil, fmt.Errorf("glb: node %q has no geometry", name)
	}
	return pos, nrm, idx, nil
}

// NodeMesh is MeshByNode plus texture coordinates and the image index of the
// node's base colour texture (-1 when it has none).
func (d *Document) NodeMesh(name string) (pos, nrm, uv []float32, idx []uint32, image int, err error) {
	node := -1
	for i := range d.Nodes {
		if d.Nodes[i].Name == name && d.Nodes[i].Mesh != nil {
			node = i
			break
		}
	}
	if node < 0 {
		return nil, nil, nil, nil, -1, fmt.Errorf("glb: no node %q with a mesh", name)
	}
	meshIdx := *d.Nodes[node].Mesh
	if meshIdx < 0 || meshIdx >= len(d.Meshes) {
		return nil, nil, nil, nil, -1, fmt.Errorf("glb: node %q mesh %d out of range", name, meshIdx)
	}
	image = -1
	for pi, prim := range d.Meshes[meshIdx].Primitives {
		posAcc, ok := prim.Attributes["POSITION"]
		if !ok {
			continue
		}
		p, comps, perr := d.Floats(posAcc)
		if perr != nil {
			return nil, nil, nil, nil, -1, perr
		}
		if comps != 3 {
			return nil, nil, nil, nil, -1, fmt.Errorf("glb: POSITION has %d components", comps)
		}
		base := uint32(len(pos) / 3)
		pos = append(pos, p...)
		if nAcc, ok := prim.Attributes["NORMAL"]; ok {
			n, ncomps, nerr := d.Floats(nAcc)
			if nerr != nil {
				return nil, nil, nil, nil, -1, nerr
			}
			if ncomps != 3 {
				return nil, nil, nil, nil, -1, fmt.Errorf("glb: NORMAL has %d components", ncomps)
			}
			nrm = append(nrm, n...)
		} else {
			nrm = append(nrm, make([]float32, len(p))...)
			for i := 0; i < len(p)/3; i++ {
				nrm[len(nrm)-len(p)+i*3+1] = 1
			}
		}
		if uvAcc, ok := prim.Attributes["TEXCOORD_0"]; ok {
			t, tcomps, terr := d.Floats(uvAcc)
			if terr == nil && tcomps == 2 {
				uv = append(uv, t...)
			} else {
				uv = append(uv, make([]float32, len(p)/3*2)...)
			}
		} else {
			uv = append(uv, make([]float32, len(p)/3*2)...)
		}
		if prim.Indices != nil {
			ii, ierr := d.Uints(*prim.Indices)
			if ierr != nil {
				return nil, nil, nil, nil, -1, ierr
			}
			for _, v := range ii {
				idx = append(idx, v+base)
			}
		} else {
			for i := uint32(0); i < uint32(len(p)/3); i++ {
				idx = append(idx, base+i)
			}
		}
		if pi == 0 && prim.Material != nil && *prim.Material >= 0 && *prim.Material < len(d.Materials) {
			m := d.Materials[*prim.Material]
			if m.PBR.BaseColorTexture != nil {
				ti := m.PBR.BaseColorTexture.Index
				if ti >= 0 && ti < len(d.Textures) && d.Textures[ti].Source != nil {
					image = *d.Textures[ti].Source
				}
			}
		}
	}
	if len(pos) == 0 || len(idx) == 0 {
		return nil, nil, nil, nil, -1, fmt.Errorf("glb: node %q has no geometry", name)
	}
	return pos, nrm, uv, idx, image, nil
}
