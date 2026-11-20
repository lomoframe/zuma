package game

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// The win screen fires a volley of ordinary beads: thirty of them, every
// palette colour, all inside a 20° cone around the aim ray and all at the same
// speed a normal shot leaves with.
func TestDryVolleyFiresThirtyNormalShots(t *testing.T) {
	g := New(7)
	o := mgl32.Vec3{0, 0, 0.2}
	d := mgl32.Vec3{0, 0.5, -1}.Normalize()

	g.DryVolley(o, d, 30)
	if len(g.Shots) != 30 {
		t.Fatalf("volley must fire 30 beads, got %d", len(g.Shots))
	}

	seen := map[int]bool{}
	limit := float32(math.Cos(volleySpread))
	for i, sh := range g.Shots {
		seen[sh.Color] = true
		if v := sh.Vel.Len(); math.Abs(float64(v-shotSpeed)) > 1e-4 {
			t.Fatalf("bead %d leaves at %.3f m/s, a normal shot is %.1f", i, v, shotSpeed)
		}
		if c := sh.Vel.Normalize().Dot(d); c < limit-1e-4 {
			t.Fatalf("bead %d leaves the cone: cos %.4f < %.4f", i, c, limit)
		}
		if start := o.Sub(sh.Pos).Len(); math.Abs(float64(start)-0.06) > 1e-4 {
			t.Fatalf("bead %d starts %.3f m from the muzzle, want 0.06", i, start)
		}
	}
	if len(seen) != len(Palette) {
		t.Fatalf("volley must use all %d colours, saw %d", len(Palette), len(seen))
	}
}

// The volley beads are ordinary shots: they fly, and they leave play under the
// same landing rule as a shot that misses, with nothing extra (no lifetime, no
// shrink step, no bounce).
func TestVolleyBeadsAreOrdinaryShots(t *testing.T) {
	g := New(11)
	o := mgl32.Vec3{0, 0.30, 0}
	g.DryVolley(o, mgl32.Vec3{0, 0.2, -1}.Normalize(), 30)

	// One frame in they have all moved along their own direction.
	before := make([]mgl32.Vec3, len(g.Shots))
	for i, sh := range g.Shots {
		before[i] = sh.Pos
	}
	g.Update(1.0 / 60.0)
	if len(g.Shots) != 30 {
		t.Fatalf("volley lost beads in the first frame: %d left", len(g.Shots))
	}
	for i, sh := range g.Shots {
		if sh.Pos == before[i] {
			t.Fatalf("bead %d did not move", i)
		}
	}

	// They keep going until they fall past the felt, exactly like a miss.
	smallest := float32(math.MaxFloat32)
	for i := 0; i < 120; i++ {
		g.Update(1.0 / 60.0)
		for _, sh := range g.Shots {
			if v := sh.Vel.Len(); v < smallest {
				smallest = v
			}
		}
	}
	if len(g.Shots) != 0 {
		t.Fatalf("volley beads never left play: %d left", len(g.Shots))
	}
	if math.Abs(float64(smallest-shotSpeed)) > 1e-4 {
		t.Fatalf("some bead slowed down in flight (%.3f m/s) — shots keep their speed", smallest)
	}
}

// The win screen waits for the player: a won round must not load the next level
// on its own no matter how long it is updated.
func TestWonStateWaitsForThePlayer(t *testing.T) {
	g := New(3)
	g.Update(2)
	wonLevel := g.Level
	g.DemoState("won")

	for i := 0; i < 600; i++ {
		g.Update(1.0 / 60.0)
	}
	if g.State != StateWon {
		t.Fatalf("win screen left StateWon after 10 s: state %v", g.State)
	}
	if g.Level != wonLevel {
		t.Fatalf("win screen advanced to level %d without a fire press", g.Level)
	}

	g.NextLevel()
	if g.State != StatePlaying || g.Level != wonLevel+1 {
		t.Fatalf("NextLevel gave state %v level %d", g.State, g.Level)
	}
}
