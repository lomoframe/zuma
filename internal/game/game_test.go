package game

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestPathArcLengthAndTangent(t *testing.T) {
	p := NewPathForBoard()
	if p.Len < 8 || p.Len > 14 {
		t.Fatalf("spiral length %.3f out of the expected range", p.Len)
	}
	prev, _ := p.At(0)
	for s := float32(0); s <= p.Len; s += 0.05 {
		pos, tan := p.At(s)
		if math.Abs(float64(tan.Len()-1)) > 1e-3 {
			t.Fatalf("tangent at %.2f is not unit: %.4f", s, tan.Len())
		}
		if d := pos.Sub(prev).Len(); d > 0.09 {
			t.Fatalf("path jumps %.4f at s=%.2f", d, s)
		}
		prev = pos
	}
	// The spiral must stay on the board.
	for s := float32(0); s <= p.Len; s += 0.02 {
		if pos := p.PointAt(s); pos.Len() > BoardRadius-0.05 {
			t.Fatalf("path leaves the board at s=%.2f: %v", s, pos)
		}
	}
}

func TestMatchRemovesContiguousRun(t *testing.T) {
	g := New(1)
	g.Balls = []Ball{
		{S: 4.00, Color: 1, ID: 1},
		{S: 3.876, Color: 0, ID: 2},
		{S: 3.752, Color: 0, ID: 3},
		{S: 3.628, Color: 0, ID: 4},
		{S: 3.504, Color: 1, ID: 5},
	}
	g.matchAt(2)
	if len(g.Balls) != 2 {
		t.Fatalf("want 2 beads left, got %d", len(g.Balls))
	}
	if g.Score != 30 {
		t.Fatalf("want 30 points, got %d", g.Score)
	}
	if g.Balls[0].ID != 1 || g.Balls[1].ID != 5 {
		t.Fatalf("wrong beads survived: %+v", g.Balls)
	}
	if g.pendingAhead != 1 {
		t.Fatalf("junction ahead not tracked: %d", g.pendingAhead)
	}
}

func TestMatchIgnoresSeparatedSameColour(t *testing.T) {
	g := New(1)
	g.Balls = []Ball{
		{S: 4.00, Color: 0, ID: 1},
		{S: 3.876, Color: 0, ID: 2},
		{S: 3.00, Color: 0, ID: 3}, // a clear gap ahead of it
	}
	g.matchAt(1)
	if len(g.Balls) != 3 {
		t.Fatalf("a run across a gap must not pop, got %d beads", len(g.Balls))
	}
}

func TestInsertPushesTailBack(t *testing.T) {
	g := New(1)
	g.Balls = []Ball{
		{S: 4.000, Color: 1, ID: 1},
		{S: 3.876, Color: 2, ID: 2},
		{S: 3.752, Color: 3, ID: 3},
		{S: 3.628, Color: 4, ID: 4},
	}
	pos, tan := g.Path.At(3.876)
	hit := mgl32.Vec3{pos.X() + tan.X()*0.04, BallR, pos.Y() + tan.Y()*0.04}
	g.insert(hit, 1, 5) // hit the front half of bead 2: goes in ahead of it
	if len(g.Balls) != 5 {
		t.Fatalf("want 5 beads, got %d", len(g.Balls))
	}
	for i := 1; i < len(g.Balls); i++ {
		if d := g.Balls[i-1].S - g.Balls[i].S; math.Abs(float64(d-Diam)) > 1e-3 {
			t.Fatalf("bead %d spacing %.4f, want %.4f", i, d, Diam)
		}
	}
	if g.Balls[1].Color != 5 {
		t.Fatalf("inserted colour %d at index 1", g.Balls[1].Color)
	}
	if g.pushFrom != 2 {
		t.Fatalf("push animation should start behind the new bead, got %d", g.pushFrom)
	}
}

func TestGapClosingTriggersChainReaction(t *testing.T) {
	g := New(1)
	g.Balls = []Ball{
		{S: 4.000, Color: 1, ID: 1},
		{S: 3.876, Color: 0, ID: 2},
		{S: 3.000, Color: 0, ID: 3},
		{S: 2.876, Color: 0, ID: 4},
	}
	g.pendingAhead = 2
	for i := 0; i < 400 && len(g.Balls) > 1; i++ {
		g.stepChain(1.0 / 60.0)
	}
	if len(g.Balls) != 1 || g.Balls[0].ID != 1 {
		t.Fatalf("chain reaction did not clear the back group: %+v", g.Balls)
	}
	if g.Combo != 1 || g.Score != 30 {
		t.Fatalf("combo/score after reaction: %d / %d", g.Combo, g.Score)
	}
}

func TestFireReloadsAndShootsAlongAim(t *testing.T) {
	g := New(7)
	origin := mgl32.Vec3{0, 1.0, 1.2}
	dir := mgl32.Vec3{0, -0.6, -0.8}.Normalize()
	loaded, next := g.Loaded, g.Next
	if !g.Fire(origin, dir) {
		t.Fatal("Fire refused while the launcher was free")
	}
	if len(g.Shots) != 1 {
		t.Fatalf("want one ball in the air, got %d", len(g.Shots))
	}
	if g.Shots[0].Color != loaded {
		t.Fatalf("shot colour %d, want the loaded %d", g.Shots[0].Color, loaded)
	}
	if g.Loaded != next {
		t.Fatalf("reload did not rotate the colours: loaded %d, next %d", g.Loaded, g.Next)
	}
	// The bead must start at the controller and travel away along the ray.
	if g.Shots[0].Pos.Sub(origin).Len() > 0.2 {
		t.Fatalf("shot spawned away from the controller: %v", g.Shots[0].Pos)
	}
	if g.Shots[0].Vel.Normalize().Dot(dir) < 0.999 {
		t.Fatalf("shot direction %v, want %v", g.Shots[0].Vel, dir)
	}
}

func TestBallReachingHoleLoses(t *testing.T) {
	g := New(1)
	g.Balls = []Ball{{S: g.Path.Len, Color: 0, ID: 1}}
	g.State = StatePlaying
	g.updateChainOnly(1.0 / 60.0)
	if g.State != StateLost {
		t.Fatalf("head in the hole should lose, state=%v", g.State)
	}
	// The chain is swallowed and leaves the board.
	for i := 0; i < 600 && len(g.Balls) > 0; i++ {
		g.Update(1.0 / 60.0)
	}
	if len(g.Balls) != 0 {
		t.Fatalf("lose animation left %d beads", len(g.Balls))
	}
}

func TestClearBoardWinsAndWaits(t *testing.T) {
	g := New(1)
	g.Balls = nil
	g.Queue = nil
	g.Shots = nil
	g.Update(1.0 / 60.0)
	if g.State != StateWon {
		t.Fatalf("empty board should win, state=%v", g.State)
	}
	lvl := g.Level
	for i := 0; i < 300; i++ {
		g.Update(1.0 / 60.0)
	}
	if g.Level != lvl || g.State != StateWon {
		t.Fatalf("win screen moved on by itself: level %d state %v", g.Level, g.State)
	}
	g.NextLevel()
	if g.Level != lvl+1 || g.State != StatePlaying {
		t.Fatalf("next level did not load: level %d state %v", g.Level, g.State)
	}
}

// updateChainOnly runs one frame of chain movement without spawning or shots,
// so a test can place beads exactly where it wants them.
func (g *Game) updateChainOnly(dt float32) {
	g.stepChain(dt)
	if g.State == StatePlaying && len(g.Balls) > 0 && g.Balls[0].S >= g.Path.Len {
		g.State = StateLost
	}
}

func TestShotPopsMatchEndToEnd(t *testing.T) {
	g := New(11)
	g.Queue = nil
	g.Shots = nil
	g.Balls = []Ball{
		{S: 5.00, Color: 2, ID: 1},
		{S: 5.00 - Diam, Color: 2, ID: 2},
	}
	g.Loaded = 2
	// Aim from above the table, the way a controller points down at the board.
	p := g.Path.PointAt(5.0)
	origin := mgl32.Vec3{p.X(), 0.95, p.Y() + 0.85}
	target := mgl32.Vec3{p.X(), BallR, p.Y()}
	if !g.Fire(origin, target.Sub(origin).Normalize()) {
		t.Fatal("fire refused")
	}
	for i := 0; i < 240; i++ {
		g.Update(1.0 / 120.0)
	}
	if len(g.Balls) != 0 {
		t.Fatalf("the run of three did not pop: %+v", g.Balls)
	}
	if g.Score < 30 {
		t.Fatalf("score %d after a three-ball pop", g.Score)
	}
	if g.State != StateWon {
		t.Fatalf("clearing the board should win, state=%v", g.State)
	}
}

func TestDemoStateLetsScreenshotsShowOverlays(t *testing.T) {
	g := New(1)
	g.DemoState("lost")
	if g.State != StateLost || !g.LoseReady() {
		t.Fatalf("lost overlay not ready: state=%v", g.State)
	}
	g.DemoState("won")
	if g.State != StateWon {
		t.Fatalf("won state not set")
	}
}

func TestComboKnockbackIsAnimated(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	g.Balls = []Ball{
		{S: 5.000, Color: 0, ID: 1},
		{S: 4.876, Color: 0, ID: 2},
		{S: 4.752, Color: 0, ID: 3},
		{S: 4.628, Color: 1, ID: 4},
	}
	g.Combo = 1 // first pop of this shot
	g.matchAt(0)
	if g.KnockVel >= 0 {
		t.Fatalf("combo did not add a backwards impulse: %v", g.KnockVel)
	}
	s0 := g.Balls[0].S
	g.stepChain(1.0 / 60.0)
	if g.Balls[0].S >= s0 {
		t.Fatalf("chain did not recoil: %.4f -> %.4f", s0, g.Balls[0].S)
	}
	v0 := g.KnockVel
	g.stepChain(1.0 / 60.0)
	if g.KnockVel <= v0 {
		t.Fatalf("recoil did not decay: %.4f -> %.4f", v0, g.KnockVel)
	}
	// Friction must settle it instead of sliding forever.
	for i := 0; i < 300; i++ {
		g.stepChain(1.0 / 60.0)
	}
	if g.KnockVel != 0 {
		t.Fatalf("recoil never settled: %v", g.KnockVel)
	}
}

func TestKnockbackRetreatsIntoTheHole(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	// A chain long enough that only its tail can reach the entrance.
	g.Balls = []Ball{
		{S: 0.05 + 4*Diam, Color: 0, ID: 1},
		{S: 0.05 + 3*Diam, Color: 1, ID: 2},
		{S: 0.05 + 2*Diam, Color: 2, ID: 3},
		{S: 0.05 + Diam, Color: 0, ID: 4},
		{S: 0.05, Color: 1, ID: 5},
	}
	g.KnockVel = -3
	retreated := false
	for i := 0; i < 240; i++ {
		g.stepChain(1.0 / 60.0)
		if g.Balls[len(g.Balls)-1].S < 0 {
			retreated = true
		}
		if g.Balls[len(g.Balls)-1].S < -knockRetreat-1e-4 {
			t.Fatalf("chain retreated past the hole: %v", g.Balls[len(g.Balls)-1].S)
		}
	}
	if !retreated {
		t.Fatalf("combo knock must push the tail into the hole, tail stayed at %v", g.Balls[len(g.Balls)-1].S)
	}
	if g.KnockVel != 0 {
		t.Fatalf("recoil never settled: %v", g.KnockVel)
	}
}

// A combo that swallows the whole chain must end the recoil at once and lay a
// fresh chain, so the table never sits empty waiting for the impulse to decay.
func TestFullChainIntoHoleRefillsImmediately(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	g.Balls = []Ball{{S: 0.02, Color: 0, ID: 1}, {S: 0.01, Color: 1, ID: 2}}
	g.KnockVel = -6
	for i := 0; i < 120; i++ {
		g.stepChain(1.0 / 60.0)
	}
	if g.KnockVel != 0 {
		t.Fatalf("recoil should stop when the chain is gone, got %v", g.KnockVel)
	}
	if len(g.Balls) < 2 {
		t.Fatalf("expected a fresh chain, got %d beads", len(g.Balls))
	}
	for _, b := range g.Balls {
		if b.S < 0 {
			t.Fatalf("fresh chain still sits inside the hole: %v", b.S)
		}
	}
}

func TestMidChainComboKnockFiresAfterCollision(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	colors := []int{1, 1, 2, 0, 0, 0, 2, 1, 1}
	g.Balls = nil
	for i, c := range colors {
		g.Balls = append(g.Balls, Ball{S: 9.0 - float32(i)*Diam, Color: c, ID: i + 1})
	}
	g.Combo = 1 // first pop of this shot
	g.matchAt(3)
	if g.pendingKnock == 0 {
		t.Fatalf("mid-chain combo did not queue a knock")
	}
	if g.KnockVel != 0 {
		t.Fatalf("knock applied before the halves met")
	}
	fired := false
	for i := 0; i < 600; i++ {
		g.stepChain(1.0 / 60.0)
		if g.KnockVel != 0 {
			fired = true
			break
		}
	}
	if !fired {
		t.Fatalf("knock never fired: pending=%v pendingAhead=%d", g.pendingKnock, g.pendingAhead)
	}
}

func TestComboKnockGrowsWithEveryPop(t *testing.T) {
	if comboImpulse(0) != 0 || comboImpulse(1) != 0 {
		t.Fatalf("knockback must start at the second pop, got %v / %v", comboImpulse(0), comboImpulse(1))
	}
	prev := comboImpulse(1)
	for c := 2; c <= 12; c++ {
		v := comboImpulse(c)
		if v <= prev {
			t.Fatalf("x%d impulse %.3f is not stronger than the previous level %.3f", c, v, prev)
		}
		if v >= knockMax {
			t.Fatalf("x%d impulse %.3f reached the ceiling, levels no longer differ", c, v)
		}
		prev = v
	}
}

func TestComboCountsPopsOfOneShotOnly(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	g.Combo = 3
	if !g.Fire(mgl32.Vec3{0, 1, 0}, mgl32.Vec3{0, 0, -1}) {
		t.Fatal("fire refused")
	}
	if g.Combo != 0 {
		t.Fatalf("a new shot must start a fresh combo count, got %d", g.Combo)
	}

	g.Balls = []Ball{
		{S: 5.000, Color: 0, ID: 11},
		{S: 4.876, Color: 0, ID: 12},
		{S: 4.752, Color: 0, ID: 13},
	}
	g.matchAt(0) // first pop of this shot
	if g.Combo != 1 {
		t.Fatalf("first pop should be x1, got %d", g.Combo)
	}

	g.Balls = []Ball{
		{S: 5.000, Color: 1, ID: 21},
		{S: 4.876, Color: 1, ID: 22},
		{S: 4.752, Color: 1, ID: 23},
	}
	g.matchAt(0) // second pop from the same shot: a real combo
	if g.Combo != 2 {
		t.Fatalf("second pop of the same shot should be x2, got %d", g.Combo)
	}
	if g.KnockVel >= 0 {
		t.Fatalf("x2 should fire a knockback, got %v", g.KnockVel)
	}

	g.Fire(mgl32.Vec3{0, 1, 0}, mgl32.Vec3{0, 0, -1})
	if g.Combo != 0 {
		t.Fatalf("next shot must reset the combo, got %d", g.Combo)
	}
}
