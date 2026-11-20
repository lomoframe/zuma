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
	g.insert(pos.Add(tan.Mul(0.04)), 1, 5) // hit the front half of bead 2: goes in ahead of it
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
	g.SetAim(mgl32.Vec2{0, -1})
	if math.Abs(float64(g.Aim.X())) > 1e-5 || math.Abs(float64(g.Aim.Y()+1)) > 1e-5 {
		t.Fatalf("aim not applied: %v", g.Aim)
	}
	loaded, next := g.Loaded, g.Next
	if !g.Fire() {
		t.Fatal("Fire refused while the frog was free")
	}
	if len(g.Shots) != 1 {
		t.Fatalf("want one ball in the air, got %d", len(g.Shots))
	}
	if g.Loaded != next {
		t.Fatalf("reload did not rotate the colours: loaded %d, next %d", g.Loaded, g.Next)
	}
	if g.Shots[0].Color != loaded {
		t.Fatalf("shot colour %d, want the loaded %d", g.Shots[0].Color, loaded)
	}
	if g.Shots[0].Vel.Y() > -1 {
		t.Fatalf("shot not aimed at the board: %v", g.Shots[0].Vel)
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

func TestClearBoardWinsAndAdvances(t *testing.T) {
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
	if g.Level != lvl+1 || g.State != StatePlaying {
		t.Fatalf("win did not advance: level %d state %v", g.Level, g.State)
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
	g.SetAim(g.Path.PointAt(5.0))
	if !g.Fire() {
		t.Fatal("fire refused")
	}
	// The frog is in the middle of the spiral, so the bead is a metre or two
	// away; half a second is plenty at 7.4 m/s.
	for i := 0; i < 120; i++ {
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
