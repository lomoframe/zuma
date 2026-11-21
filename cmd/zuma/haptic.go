package main

import (
	"zuma/internal/game"
	"zuma/internal/xr"
)

// hapticPulse is one rumble: when to fire it (seconds after the gameplay moment
// that triggered it), which hand, and the vibration itself.
type hapticPulse struct {
	at   float64
	hand int
	amp  float32
	hz   float32
	secs float32
}

// hapticFor maps a gameplay moment to its rumble. Each kind is given its own
// amplitude, pitch and length so they stay distinguishable with the headset on:
//
//	shoot   a short tick under the trigger finger
//	insert  a crisper click, the bead locking into the chain
//	pop     longer and lower, deepening with the combo
//	knock   a two-handed thump with a short aftershock, the whole chain moving
//	win     a rising four-note arpeggio in both hands
//
// fireHand is the launcher that owns the bead; impacts and the celebration use
// both hands because the player feels those with the whole body.
func hapticFor(kind game.EventKind, value, fireHand int) []hapticPulse {
	switch kind {
	case game.EventShoot:
		return []hapticPulse{{hand: fireHand, amp: 0.20, hz: 130, secs: 0.030}}

	case game.EventInsert:
		return []hapticPulse{{hand: fireHand, amp: 0.32, hz: 95, secs: 0.028}}

	case game.EventMatch:
		c := value
		if c < 1 {
			c = 1
		}
		amp := 0.45 + 0.05*float32(c-1)
		if amp > 0.85 {
			amp = 0.85
		}
		hz := 72 - 4*float32(c-1)
		if hz < 45 {
			hz = 45
		}
		secs := 0.070 + 0.008*float32(c)
		if secs > 0.150 {
			secs = 0.150
		}
		out := []hapticPulse{{hand: fireHand, amp: amp, hz: hz, secs: secs}}
		if c >= 2 {
			// A second, softer pulse close behind it: a double hit reads as
			// "that counted more than once".
			out = append(out, hapticPulse{at: 0.055, hand: fireHand, amp: amp * 0.55, hz: hz, secs: secs * 0.6})
		}
		return out

	case game.EventKnock:
		return []hapticPulse{
			{hand: xr.Left, amp: 0.75, hz: 45, secs: 0.110},
			{hand: xr.Right, amp: 0.75, hz: 45, secs: 0.110},
			{at: 0.060, hand: xr.Left, amp: 0.40, hz: 38, secs: 0.070},
			{at: 0.060, hand: xr.Right, amp: 0.40, hz: 38, secs: 0.070},
		}

	case game.EventWin:
		var out []hapticPulse
		for i := 0; i < 4; i++ {
			amp := 0.26 + 0.09*float32(i)
			hz := 85 + 30*float32(i)
			for _, hand := range []int{xr.Left, xr.Right} {
				out = append(out, hapticPulse{
					at:   0.10 * float64(i),
					hand: hand,
					amp:  amp,
					hz:   hz,
					secs: 0.090,
				})
			}
		}
		return out
	}
	return nil
}

// queueHaptic schedules the rumbles for one gameplay event.
func (a *app) queueHaptic(now float64, kind game.EventKind, value int) {
	for _, p := range hapticFor(kind, value, a.fireHand) {
		a.haptics = append(a.haptics, hapticPulse{
			at:   now + p.at,
			hand: p.hand,
			amp:  p.amp,
			hz:   p.hz,
			secs: p.secs,
		})
	}
}

// runHaptics fires the pulses that have come due. Call it once per frame after
// the game has been updated, so a shot fired this frame buzzes this frame.
func (a *app) runHaptics(now float64) {
	if len(a.haptics) == 0 {
		return
	}
	due := a.haptics[:0]
	for _, p := range a.haptics {
		if now < p.at {
			due = append(due, p)
			continue
		}
		if a.xrOn {
			xr.Haptic(p.hand, p.amp, p.hz, p.secs)
		}
	}
	a.haptics = due
}
