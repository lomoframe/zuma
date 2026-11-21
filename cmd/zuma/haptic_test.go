package main

import (
	"testing"

	"zuma/internal/game"
	"zuma/internal/xr"
)

// TestEachEventHasItsOwnRumble pins the feel: every gameplay beat the player
// asked for gets a pulse, the single-hand ones stay on the launcher, and the
// impact/celebration ones reach both hands.
func TestEachEventHasItsOwnRumble(t *testing.T) {
	const fire = xr.Right
	cases := []game.EventKind{game.EventShoot, game.EventInsert, game.EventMatch, game.EventKnock, game.EventWin}
	for _, kind := range cases {
		if p := hapticFor(kind, 1, fire); len(p) == 0 {
			t.Fatalf("event %d has no rumble", kind)
		}
	}
	for _, kind := range []game.EventKind{game.EventShoot, game.EventInsert, game.EventMatch} {
		for _, p := range hapticFor(kind, 1, fire) {
			if p.hand != fire {
				t.Fatalf("event %d rumbled the wrong hand %d", kind, p.hand)
			}
		}
	}
	for _, kind := range []game.EventKind{game.EventKnock, game.EventWin} {
		hands := map[int]bool{}
		for _, p := range hapticFor(kind, 2, fire) {
			hands[p.hand] = true
		}
		if !hands[xr.Left] || !hands[xr.Right] {
			t.Fatalf("event %d should reach both hands, got %v", kind, hands)
		}
	}
	// The other hand must be able to hold the launcher too.
	for _, p := range hapticFor(game.EventShoot, 1, xr.Left) {
		if p.hand != xr.Left {
			t.Fatalf("shoot followed fireHand=%d but rumbled %d", xr.Left, p.hand)
		}
	}
}

// TestComboRumbleGrows keeps the pop feedback graded: a louder, longer, lower
// buzz as the combo climbs, without ever going out of range.
func TestComboRumbleGrows(t *testing.T) {
	prevAmp, prevSecs := float32(-1), float32(-1)
	prevHz := float32(1e9)
	for c := 1; c <= 9; c++ {
		p := hapticFor(game.EventMatch, c, xr.Right)
		if len(p) == 0 {
			t.Fatalf("combo %d has no rumble", c)
		}
		first := p[0]
		if first.amp <= prevAmp {
			t.Fatalf("combo %d amp %v did not grow past %v", c, first.amp, prevAmp)
		}
		if first.secs < prevSecs {
			t.Fatalf("combo %d length %v shrank from %v", c, first.secs, prevSecs)
		}
		if first.hz > prevHz {
			t.Fatalf("combo %d pitch %v rose above %v", c, first.hz, prevHz)
		}
		if first.amp > 1 || first.hz < 20 || first.secs <= 0 {
			t.Fatalf("combo %d pulse out of range: %+v", c, first)
		}
		prevAmp, prevSecs, prevHz = first.amp, first.secs, first.hz
	}
}

// TestHapticsFireWhenDue checks the scheduler: delayed pulses wait, due ones are
// consumed exactly once.
func TestHapticsFireWhenDue(t *testing.T) {
	a := &app{fireHand: xr.Right}
	now := 100.0
	a.queueHaptic(now, game.EventKnock, 3)
	if len(a.haptics) == 0 {
		t.Fatal("knock queued nothing")
	}
	total := len(a.haptics)

	a.runHaptics(now) // the two immediate pulses are due, the aftershocks are not
	left := len(a.haptics)
	if left == 0 || left >= total {
		t.Fatalf("expected only the immediate pulses to fire, kept %d of %d", left, total)
	}
	a.runHaptics(now)
	if len(a.haptics) != left {
		t.Fatal("a drained pulse fired twice")
	}
	a.runHaptics(now + 1.0)
	if len(a.haptics) != 0 {
		t.Fatalf("delayed pulses never fired: %d left", len(a.haptics))
	}
}

// TestUnknownEventIsSilent keeps level/lose events (which have no rumble design)
// from buzzing by accident.
func TestUnknownEventIsSilent(t *testing.T) {
	for _, kind := range []game.EventKind{game.EventLose, game.EventLevel} {
		if p := hapticFor(kind, 1, xr.Right); len(p) != 0 {
			t.Fatalf("event %d should not rumble, got %v", kind, p)
		}
	}
}
