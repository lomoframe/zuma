package audio

import "testing"

// TestClipLengths pins the duration of every synthesised clip so a refactor
// cannot silently shorten the celebration or the firework tail.
func TestClipLengths(t *testing.T) {
	r := &noise{state: 22222}
	cases := []struct {
		name string
		pcm  []float32
		want int
	}{
		{"firework", synthFirework(r), 35280},
		{"landing", synthLanding(r), 6174},
		{"boost", synthBoost(r), 12127},
		{"wind", synthWind(r), 26460},
		{"cheer", synthCheer(), 66150},
		{"accel", synthAccel(r), 9922},
		{"brake", synthBrake(r), 7497},
		{"footstep", synthFootstep(r), 2646},
		{"engine", synthEngine(r), 11025},
		{"water", synthWater(r), 22050},
		{"splash", synthSplash(r), 9922},
		{"crash", synthCrash(r), 19845},
		{"startle", synthStartle(r), 6615},
		{"ignition", synthIgnition(r), 24255},
		{"scream", synthScream(r), 12127},
		{"neigh", synthNeigh(r), 22050},
		{"hoof", synthHoof(r), 2646},
	}
	for _, c := range cases {
		if got := len(c.pcm); got != c.want {
			t.Errorf("%s: %d samples, want %d", c.name, got, c.want)
		}
	}
}

// TestClipPeaks checks the normalised clips hit their target peak and the rest
// stay inside the 16-bit range.
func TestClipPeaks(t *testing.T) {
	r := &noise{state: 22222}
	cases := []struct {
		name   string
		pcm    []float32
		target float32 // 0 means "just stay in range"
	}{
		{"firework", synthFirework(r), 0.85},
		{"landing", synthLanding(r), 0},
		{"boost", synthBoost(r), 0},
		{"wind", synthWind(r), 0},
		{"cheer", synthCheer(), 0.80},
		{"accel", synthAccel(r), 0.60},
		{"brake", synthBrake(r), 0.55},
		{"footstep", synthFootstep(r), 0},
		{"engine", synthEngine(r), 0.75},
		{"water", synthWater(r), 0.60},
		{"splash", synthSplash(r), 0.70},
		{"crash", synthCrash(r), 0.85},
		{"startle", synthStartle(r), 0.55},
		{"ignition", synthIgnition(r), 0.80},
		{"scream", synthScream(r), 0.90},
		{"neigh", synthNeigh(r), 0.85},
		{"hoof", synthHoof(r), 0.70},
	}
	for _, c := range cases {
		peak := float32(0)
		for _, v := range c.pcm {
			if v != v { // NaN
				t.Fatalf("%s: NaN sample", c.name)
			}
			// Unnormalised clips are clamped to 16-bit on upload; the
			// normalised ones must land exactly on their target.
			if c.target > 0 && (v < -1 || v > 1) {
				t.Fatalf("%s: sample %v outside [-1, 1]", c.name, v)
			}
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		if peak == 0 {
			t.Fatalf("%s: silent clip", c.name)
		}
		if c.target > 0 && (peak-c.target > 1e-4 || c.target-peak > 1e-4) {
			t.Errorf("%s: peak %v, want %v", c.name, peak, c.target)
		}
	}
}

// TestSynthesisIsDeterministic keeps the clips identical between runs.
func TestSynthesisIsDeterministic(t *testing.T) {
	a := synthFirework(&noise{state: 22222})
	b := synthFirework(&noise{state: 22222})
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs: %v != %v", i, a[i], b[i])
		}
	}
}

func TestToPCMClamps(t *testing.T) {
	got := toPCM([]float32{2, -2, 0, 0.5})
	want := []int16{32000, -32000, 0, 16000}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d: got %d, want %d", i, got[i], want[i])
		}
	}
}
