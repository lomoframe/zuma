package render

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"zuma/internal/game"
)

// TestBuilderWinding checks that every primitive's triangles face the same way
// as their vertex normals. Back-face culling hides an inverted primitive, so a
// winding mistake is invisible in code and obvious only on screen.
func TestBuilderWinding(t *testing.T) {
	b := &Builder{}
	b.sphere(mgl32.Vec3{0, 0, 0}, 1, rgba(1, 1, 1, 1), 16, 8)
	b.cylinder(0, 0, 0.5, 0, 1, rgba(1, 1, 1, 1), 12)
	b.disc(0, 0, 1, 0, rgba(1, 1, 1, 1), 16, true)
	b.disc(0, 0, 1, 0, rgba(1, 1, 1, 1), 16, false)
	b.torus(0, 0, 1, 0.2, 0, rgba(1, 1, 1, 1), 16, 8)
	b.flatQuad(mgl32.Vec3{-1, 0, 0}, mgl32.Vec3{1, 0, 0}, mgl32.Vec3{1, 1, 0}, mgl32.Vec3{-1, 1, 0}, rgba(1, 1, 1, 1))
	b.ribbon(game.NewPathForBoard(), 0.2, 0, rgba(1, 1, 1, 1))

	bad := 0
	for i := 0; i+2 < len(b.idx); i += 3 {
		a := b.verts[b.idx[i]]
		c := b.verts[b.idx[i+1]]
		d := b.verts[b.idx[i+2]]
		n := c.Pos.Sub(a.Pos).Cross(d.Pos.Sub(a.Pos))
		if n.Len() < 1e-7 {
			continue // degenerate pole triangle
		}
		n = n.Normalize()
		avg := a.Nrm.Add(c.Nrm).Add(d.Nrm)
		if avg.Len() < 1e-6 {
			continue
		}
		if n.Dot(avg.Normalize()) < 0 {
			bad++
		}
	}
	if bad > 0 {
		t.Fatalf("%d triangles wind against their normals", bad)
	}
}

func TestBoardAndFrogBuild(t *testing.T) {
	board := buildBoard(game.CircleBoard(3.10), game.NewPathForBoard())
	if len(board.idx) < 3000 {
		t.Fatalf("board mesh looks empty: %d indices", len(board.idx))
	}
	frog := buildFrog()
	if len(frog.idx) < 1000 {
		t.Fatalf("frog mesh looks empty: %d indices", len(frog.idx))
	}
}
