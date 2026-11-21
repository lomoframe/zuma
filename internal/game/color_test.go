package game

import "testing"

// TestHandOnlyGetsLiveColours checks the launcher never offers a colour that
// has been cleared from the chain: such a bead could not match anything.
func TestHandOnlyGetsLiveColours(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	g.Balls = []Ball{
		{S: 5.000, Color: 0, ID: 1},
		{S: 4.876, Color: 2, ID: 2},
	}
	for i := 0; i < 400; i++ {
		if c := g.pickColor(4); c != 0 && c != 2 {
			t.Fatalf("handed out colour %d, which is not on the chain", c)
		}
	}
}

// TestHandMixesEveryColourOnAnEmptyBoard keeps the endgame playable: with no
// chain left the pick falls back to any colour.
func TestHandMixesEveryColourOnAnEmptyBoard(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	g.Balls = nil
	seen := map[int]bool{}
	for i := 0; i < 400; i++ {
		seen[g.pickColor(4)] = true
	}
	if len(seen) != 4 {
		t.Fatalf("empty board should still mix all colours, got %v", seen)
	}
}

// TestHandDropsClearedColour covers the loaded bead: once its colour is gone
// from the board the hand must switch to a colour that can still match.
func TestHandDropsClearedColour(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Shots = nil
	g.Balls = []Ball{{S: 5.000, Color: 1, ID: 1}}
	g.Loaded = 3
	g.Update(1.0 / 60.0)
	if g.Loaded != 1 {
		t.Fatalf("hand kept the cleared colour %d", g.Loaded)
	}
}

// TestBallInFlightCountsAsLive lets a bead that is still travelling keep its
// colour available for the next handful.
func TestBallInFlightCountsAsLive(t *testing.T) {
	g := New(1)
	g.Queue = nil
	g.Balls = []Ball{{S: 5.000, Color: 0, ID: 1}}
	g.Shots = []Shot{{Color: 3}}
	g.Loaded = 0
	g.Update(1.0 / 60.0)
	if g.Loaded != 0 {
		// A shot of colour 3 is still usable, so 0 and 3 are both legit; just
		// make sure a dead colour (1, 2) never shows up.
		if g.Loaded != 3 {
			t.Fatalf("hand picked colour %d, which is neither on the chain nor in flight", g.Loaded)
		}
	}
}

// TestQueuedBeadsCountAsLive keeps colours that are still in the hole alive:
// after a full-chain retreat the table is empty but the beads are queued, so
// the launcher must not start dealing from a reduced palette.
func TestQueuedBeadsCountAsLive(t *testing.T) {
	g := New(1)
	g.Shots = nil
	g.Balls = nil
	g.Queue = []int{1, 3}
	for i := 0; i < 400; i++ {
		if c := g.pickColor(4); c != 1 && c != 3 {
			t.Fatalf("handed out colour %d, which is neither on the chain nor queued", c)
		}
	}
	// Directly: a colour that only exists in the queue counts as live, and one
	// that exists nowhere does not.
	g.Balls = nil
	g.Shots = nil
	g.Queue = []int{2}
	if !g.colourOnBoard(2, 4) {
		t.Fatalf("queued colour 2 reported dead")
	}
	if g.colourOnBoard(0, 4) {
		t.Fatalf("colour 0 is nowhere and reported live")
	}
}
