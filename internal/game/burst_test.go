package game

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// The win screen fires a party-popper burst of real beads: thirty of them, every
// palette colour, all inside a 20° cone around the aim ray.
func TestLaunchBurstThrowsEveryColourInsideTheCone(t *testing.T) {
	g := New(7)
	o := mgl32.Vec3{0, 0, 0.2}
	d := mgl32.Vec3{0, 0.5, -1}.Normalize()

	g.LaunchBurst(o, d, 30)
	if len(g.Sparks) != 30 {
		t.Fatalf("burst must throw 30 beads, got %d", len(g.Sparks))
	}

	seen := map[int]bool{}
	limit := float32(math.Cos(sparkSpread))
	for i, sp := range g.Sparks {
		seen[sp.Colour] = true
		if sp.Pos != o {
			t.Fatalf("spark %d starts at %v, want %v", i, sp.Pos, o)
		}
		dir := sp.Vel.Normalize()
		if c := dir.Dot(d); c < limit-1e-4 {
			t.Fatalf("spark %d leaves the cone: cos %.4f < %.4f", i, c, limit)
		}
		if s := sp.Vel.Len(); s < sparkSpeedLo-1e-3 || s > sparkSpeedHi+1e-3 {
			t.Fatalf("spark %d speed %.3f outside [%.1f, %.1f]", i, s, sparkSpeedLo, sparkSpeedHi)
		}
	}
	if len(seen) != len(Palette) {
		t.Fatalf("burst must use all %d colours, saw %d", len(Palette), len(seen))
	}
}

// Sparks fly and die: after the lifetime the burst is gone, and the beads have
// moved away from the muzzle in the meantime.
func TestSparksFlyApartAndExpire(t *testing.T) {
	g := New(11)
	o := mgl32.Vec3{0, 0.3, 0}
	g.LaunchBurst(o, mgl32.Vec3{0, 0.2, -1}.Normalize(), 30)

	for i := 0; i < 15; i++ {
		g.Update(1.0 / 60.0)
	}
	if len(g.Sparks) != 30 {
		t.Fatalf("burst died early: %d beads left", len(g.Sparks))
	}
	moved := 0
	for _, sp := range g.Sparks {
		if sp.Pos.Sub(o).Len() > 0.3 {
			moved++
		}
	}
	if moved < 25 {
		t.Fatalf("only %d of 30 beads travelled, they are not flying", moved)
	}

	for i := 0; i < 200; i++ {
		g.Update(1.0 / 60.0)
	}
	if len(g.Sparks) != 0 {
		t.Fatalf("burst should be spent after %.1f s, %d left", SparkLife, len(g.Sparks))
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
