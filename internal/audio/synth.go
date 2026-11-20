package audio

import "math"

// Every clip is synthesised here at start up, so the game ships no audio
// assets. The parameters were tuned by ear; changing them changes the sound.
const (
	sampleRate = 22050
	twoPi      = 2 * math.Pi
)

// noise is a deterministic LCG, so every run hears the same clips and a long
// firework show keeps the same sparkle pattern.
type noise struct{ state uint32 }

func (n *noise) next() float32 {
	n.state = n.state*1664525 + 1013904223
	return float32((n.state>>8)&0xffffff)/8388608 - 1
}

// samples converts a duration in seconds to a sample count.
func samples(seconds float32) int { return int(float32(sampleRate) * seconds) }

func sinf(x float32) float32 { return float32(math.Sin(float64(x))) }
func expf(x float32) float32 { return float32(math.Exp(float64(x))) }

// synthFirework renders a real shell going off: a bright crack, a deep boom
// that drops in pitch, a low rumble and a tail of spark crackle. No chime: this
// one is meant to sound like the real thing.
func synthFirework(r *noise) []float32 {
	const dur = float32(1.6)
	n := samples(dur)
	pcm := make([]float32, n)

	// Body of the burst: falling-pitch boom, low-passed noise punch and a sub
	// rumble underneath.
	var lp, lp2 float32
	for i := range pcm {
		t := float32(i) / sampleRate

		// The crack right at the flash: a very short bright burst.
		crack := r.next() * expf(-110*t) * 0.55

		// Boom: 130 Hz falling to ~50 Hz, quick attack, long decay.
		f := 50 + 80*expf(-9*t)
		boom := sinf(twoPi*f*t) * expf(-3.4*t) * 0.85

		// Punch: low-passed noise gives the "thump" weight.
		lp += 0.13 * (r.next() - lp)
		punch := lp * 2.6 * expf(-5*t)

		// Rumble: sub-bass that hangs around after the hit.
		rumble := sinf(twoPi*46*t) * expf(-2.2*t) * 0.34

		// Ash tail: bright noise fading slowly, the sound of sparks falling.
		lp2 += 0.55 * (r.next() - lp2)
		ash := (r.next() - lp2) * 0.16 * expf(-1.8*t)

		pcm[i] = crack + boom + punch + rumble + ash
	}

	// Spark crackle: tiny bright clicks scattered through the tail, denser just
	// after the burst and thinning out as the sparks die.
	for c := 0; c < 46; c++ {
		u := 0.5 * (r.next() + 1)
		start := float32(0.10) + dur*u*u*0.55
		s0 := int(start * sampleRate)
		ln := samples(0.006)
		if ln < 2 {
			ln = 2
		}
		amp := 0.25 + 0.75*0.5*(r.next()+1)
		var hp float32
		for k := 0; k < ln && s0+k < n; k++ {
			e := expf(-6 * float32(k) / float32(ln))
			x := r.next()
			hp = 0.35*hp + 0.65*x
			pcm[s0+k] += (x - hp) * e * amp * 0.55
		}
	}

	// A little outdoor space so the boom does not stop dead.
	d := samples(0.09)
	for i := n - 1; i >= d; i-- {
		pcm[i] += 0.22 * pcm[i-d]
	}

	for i := range pcm {
		t := float32(i) / sampleRate
		if t > dur-0.25 {
			pcm[i] *= (dur - t) / 0.25
		}
	}
	normalize(pcm, 0.85)
	return pcm
}

// synthLanding is a short low thud with a soft noise edge.
func synthLanding(r *noise) []float32 {
	const dur = float32(0.28)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		f := 105 - 45*(t/dur)
		body := sinf(twoPi*f*t) * expf(-14*t)
		lp += 0.25 * (r.next() - lp)
		edge := lp * 1.4 * expf(-40*t)
		pcm[i] = 0.85*body + 0.3*edge
	}
	return pcm
}

// synthFootstep is a very short, soft heel-to-toe thump for walking. It is
// deliberately quieter and duller than the landing clip.
func synthFootstep(r *noise) []float32 {
	const dur = float32(0.12)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		f := 88 - 28*u
		body := sinf(twoPi*f*t) * expf(-32*t) * 0.55
		lp += 0.32 * (r.next() - lp)
		edge := lp * 0.9 * expf(-52*t)
		pcm[i] = body + 0.22*edge
	}
	return pcm
}

// synthBoost is a rising whoosh, like air rushing past a ring.
func synthBoost(r *noise) []float32 {
	const dur = float32(0.55)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		env := float32(1)
		if u < 0.12 {
			env = u / 0.12
		}
		env *= expf(-3 * maxf(u-0.12, 0))
		// Cutoff rises with the sweep, so the noise brightens as it passes.
		cut := 0.04 + 0.22*u
		lp += cut * (r.next() - lp)
		sweep := sinf(twoPi*(180+620*u)*t) * 0.35
		pcm[i] = env * (lp*2.2 + sweep)
	}
	return pcm
}

// synthWind is filtered noise with a slow swell, cross-faded into a seamless
// loop.
func synthWind(r *noise) []float32 {
	const dur = float32(1.2)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		lp += 0.10 * (r.next() - lp)
		swell := 0.72 + 0.28*sinf(twoPi*1.6*t)
		pcm[i] = lp * 2.2 * swell
	}
	loopify(pcm, samples(0.08))
	return pcm
}

// synthCheer is a three-second fanfare for the moment a course is cleared.
// Purely musical: a bell arpeggio blooming into a held chord over a low fifth,
// with a short echo. Crowd noise was tried first and reads as hiss, not
// applause.
func synthCheer() []float32 {
	const dur = float32(3.0)
	n := samples(dur)
	pcm := make([]float32, n)
	arp := [4]float32{523.25, 659.26, 783.99, 1046.50}

	// Fanfare: C5 E5 G5 C6, each note a soft bell (fundamental plus two
	// partials) that rings on after the next one starts.
	for note := 0; note < 4; note++ {
		start := 0.04 + 0.15*float32(note)
		s0 := int(start * sampleRate)
		ln := samples(1.5)
		f := arp[note]
		for k := 0; k < ln && s0+k < n; k++ {
			t := float32(k) / sampleRate
			attack := float32(1)
			if t < 0.012 {
				attack = t / 0.012
			}
			env := attack * expf(-2.4*t-0.7*t*t)
			v := sinf(twoPi*f*t) + 0.30*sinf(twoPi*2*f*t) + 0.12*sinf(twoPi*3*f*t)
			pcm[s0+k] += v * env * 0.17
		}
	}

	// Chord bloom: the arpeggio lands on a held C major, so the second half of
	// the cue has body instead of fading into the pad alone.
	for note := 0; note < 4; note++ {
		f := arp[note]
		s0 := samples(0.62)
		ln := samples(2.2)
		for k := 0; k < ln && s0+k < n; k++ {
			t := float32(k) / sampleRate
			attack := float32(1)
			if t < 0.06 {
				attack = t / 0.06
			}
			env := attack * expf(-1.1*t)
			v := sinf(twoPi*f*t) + 0.22*sinf(twoPi*2*f*t)
			pcm[s0+k] += v * env * 0.085
		}
	}

	// Warm bed: G3 and D4 a fifth apart, swelling in under the bells.
	for _, f := range [2]float32{196.00, 293.66} {
		for i := range pcm {
			t := float32(i) / sampleRate
			env := float32(1)
			if t < 0.25 {
				env = t / 0.25
			}
			if t > dur-0.9 {
				env *= (dur - t) / 0.9
			}
			if env < 0 {
				env = 0
			}
			pcm[i] += sinf(twoPi*f*t) * env * 0.055
		}
	}

	// A touch of room: one short delayed copy, so the bells do not stop dead.
	d := samples(0.075)
	for i := n - 1; i >= d; i-- {
		pcm[i] += 0.24 * pcm[i-d]
	}

	// Fade in and out, then normalise so nothing clips.
	for i := range pcm {
		t := float32(i) / sampleRate
		if t < 0.05 {
			pcm[i] *= t / 0.05
		}
		if t > dur-0.55 {
			pcm[i] *= (dur - t) / 0.55
		}
	}
	normalize(pcm, 0.80)
	return pcm
}

// synthAccel is the short rising whoosh when the rider pushes the throttle
// forward: a rising whine under brightening filtered noise.
func synthAccel(r *noise) []float32 {
	const dur = float32(0.45)
	pcm := make([]float32, samples(dur))
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		f := 120 + 520*u*u
		whine := sinf(twoPi*f*t) * 0.22
		lp += (0.05 + 0.25*u) * (r.next() - lp)
		attack := u / 0.05
		if attack > 1 {
			attack = 1
		}
		env := attack * (1 - 0.25*u)
		pcm[i] = (lp*1.1 + whine) * env
	}
	normalize(pcm, 0.60)
	return pcm
}

// synthBrake is the falling hiss when the rider pulls the throttle back.
func synthBrake(r *noise) []float32 {
	const dur = float32(0.34)
	pcm := make([]float32, samples(dur))
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		f := 520 - 400*u
		lp += 0.25 * (r.next() - lp)
		attack := u / 0.02
		if attack > 1 {
			attack = 1
		}
		env := attack * expf(-2.5*u)
		pcm[i] = (lp*1.2 + sinf(twoPi*f*t)*0.18) * env
	}
	normalize(pcm, 0.55)
	return pcm
}

// synthEngine is the motorcycle's idle-to-speed loop: a 26 Hz firing pulse
// with harmonics over a filtered intake rumble. It is 0.5 s long, exactly 13
// firing cycles, so looping it does not beat against the pulse.
func synthEngine(r *noise) []float32 {
	const dur = float32(0.5)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp, hp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		phase := math.Mod(float64(t*26), 1)
		fire := float32(math.Exp(-9 * phase))
		body := sinf(twoPi*26*t)*0.55 + sinf(twoPi*52*t)*0.30 + sinf(twoPi*78*t)*0.12
		lp += 0.07 * (r.next() - lp)
		hp += 0.45 * (r.next() - hp)
		// Intake hiss rides the pulse; the lowpassed layer gives the thump.
		pcm[i] = body*0.55*(0.55+0.45*fire) + lp*1.6 + (r.next()-hp)*0.07*fire
	}
	loopify(pcm, samples(0.08))
	normalize(pcm, 0.75)
	return pcm
}

// synthWater is the shallow-water loop while the bike crosses the lake:
// band-limited rushing noise with scattered bubbles.
func synthWater(r *noise) []float32 {
	const dur = float32(1.0)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp, hp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		x := r.next()
		lp += 0.20 * (x - lp)
		hp += 0.015 * (x - hp)
		swell := 0.72 + 0.28*sinf(twoPi*3.1*t)
		pcm[i] = (lp - hp) * 2.3 * swell
	}
	// Bubbles: short rising blips, written with wrap-around so the loop seam
	// never cuts one in half.
	for b := 0; b < 20; b++ {
		start := 0.5 * (r.next() + 1) * (dur - 0.12)
		s0 := int(start * sampleRate)
		ln := samples(0.02 + 0.04*0.5*(r.next()+1))
		f := 260 + 760*0.5*(r.next()+1)
		for k := 0; k < ln; k++ {
			t := float32(k) / sampleRate
			env := float32(math.Exp(-70 * float64(t)))
			pcm[(s0+k)%n] += sinf(twoPi*f*t) * env * 0.18
		}
	}
	loopify(pcm, samples(0.12))
	normalize(pcm, 0.60)
	return pcm
}

// synthSplash is the spray burst when a bike chops into the water: a bright
// noise hit that darkens as it decays, plus a handful of droplet plips.
func synthSplash(r *noise) []float32 {
	const dur = float32(0.45)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		lp += (0.30 - 0.22*u) * (r.next() - lp)
		attack := u / 0.01
		if attack > 1 {
			attack = 1
		}
		env := attack * expf(-5.5*u)
		pcm[i] = lp * 2.6 * env
	}
	for d := 0; d < 9; d++ {
		s0 := int((0.03 + 0.30*0.5*(r.next()+1)) * sampleRate)
		ln := samples(0.02 + 0.03*0.5*(r.next()+1))
		f := 500 + 900*0.5*(r.next()+1)
		for k := 0; k < ln && s0+k < n; k++ {
			t := float32(k) / sampleRate
			pcm[s0+k] += sinf(twoPi*f*t) * float32(math.Exp(-90*float64(t))) * 0.12
		}
	}
	normalize(pcm, 0.70)
	return pcm
}

// synthCrash is a motorcycle hitting a tree: a crunch of broadband noise, three
// detuned metal partials ringing out and a low thud underneath.
func synthCrash(r *noise) []float32 {
	const dur = float32(0.9)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	metal := [3]float32{322, 547, 871}
	for i := range pcm {
		t := float32(i) / sampleRate
		lp += 0.55 * (r.next() - lp)
		crunch := (r.next() - lp) * 1.5 * expf(-16*t)
		ring := float32(0)
		for k, f := range metal {
			ring += sinf(twoPi*f*t) * expf(-(4.5+1.5*float32(k))*t) * 0.18
		}
		thud := sinf(twoPi*(62-18*t/dur)*t) * expf(-6*t) * 0.55
		// A few sharper knocks as the bike tumbles.
		pcm[i] = crunch + ring + thud
	}
	for k := 0; k < 5; k++ {
		s0 := int((0.06 + 0.5*0.5*(r.next()+1)) * sampleRate)
		ln := samples(0.02)
		amp := 0.25 + 0.35*0.5*(r.next()+1)
		for j := 0; j < ln && s0+j < n; j++ {
			pcm[s0+j] += r.next() * amp * expf(-40*float32(j)/float32(ln))
		}
	}
	normalize(pcm, 0.85)
	return pcm
}

// synthStartle is a small animal's alarm cry: a quick rising squeal with a
// wobble and a breath of noise. It is short so a herd can chirp over itself.
func synthStartle(r *noise) []float32 {
	const dur = float32(0.3)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		f := 520 + 620*u + 45*sinf(twoPi*23*t)
		attack := u / 0.02
		if attack > 1 {
			attack = 1
		}
		env := attack * expf(-3.4*u)
		lp += 0.3 * (r.next() - lp)
		pcm[i] = (sinf(twoPi*f*t)*0.45 + sinf(twoPi*2*f*t)*0.12 + lp*0.5) * env
	}
	normalize(pcm, 0.55)
	return pcm
}

// synthIgnition is starting the bike: half a second of starter cranking and
// gear whine, then the engine catches and revs up as the loop takes over.
func synthIgnition(r *noise) []float32 {
	const dur = float32(1.1)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	var phase float64
	for i := range pcm {
		t := float32(i) / sampleRate
		x := r.next()
		lp += 0.25 * (x - lp)
		if t < 0.5 {
			// Starter motor: seven cranks a second, each a low compression thud
			// with a gear whine over it.
			crank := float32(math.Exp(-9 * math.Mod(float64(t*7), 1)))
			whine := sinf(twoPi*(260+40*sinf(twoPi*7*t))*t) * 0.10
			body := sinf(twoPi*42*t)*0.35 + lp*0.8
			pcm[i] = (body + whine) * crank * 0.9
			continue
		}
		// Engine catches: the firing rate climbs from 10 Hz to 55 Hz.
		tt := t - 0.5
		f := 10 + 45*minf(tt/0.45, 1)
		phase += float64(f / sampleRate)
		fire := float32(math.Exp(-6 * math.Mod(phase, 1)))
		body := sinf(twoPi*float32(phase))*0.55 + sinf(2*twoPi*float32(phase))*0.25
		env := minf(tt/0.03, 1) * (0.55 + 0.45*float32(math.Exp(-1.5*float64(tt))))
		pcm[i] = (body + lp*0.9) * fire * env * 0.85
	}
	// Fade the last 60 ms out so the engine loop can take over without a click.
	fade := samples(0.06)
	for i := 0; i < fade && i < n; i++ {
		pcm[n-1-i] *= float32(i) / float32(fade)
	}
	normalize(pcm, 0.80)
	return pcm
}

// synthScream is a small animal's cry of pain when the bike hits it: a bright,
// falling squeal with a fast wobble and a rasp of noise, much harsher than the
// startle chirp.
func synthScream(r *noise) []float32 {
	const dur = float32(0.55)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		f := 1050 - 620*u + 60*sinf(twoPi*17*t)
		attack := u / 0.008
		if attack > 1 {
			attack = 1
		}
		env := attack * expf(-3.2*u)
		lp += 0.35 * (r.next() - lp)
		pcm[i] = (sinf(twoPi*f*t)*0.45 + sinf(twoPi*2*f*t)*0.20 +
			sinf(twoPi*3*f*t)*0.10 + lp*0.35) * env
	}
	normalize(pcm, 0.90)
	return pcm
}

// synthNeigh is the horse's whinny when the rider climbs on: a rising then
// falling squeal with a fast tremolo and a breathy rasp.
func synthNeigh(r *noise) []float32 {
	const dur = float32(1.0)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		rise := u / 0.3
		if rise > 1 {
			rise = 1
		}
		f := 520 + 380*rise - 180*u
		vib := 1 + 0.06*sinf(twoPi*9*t)
		attack := u / 0.02
		if attack > 1 {
			attack = 1
		}
		env := attack * expf(-1.8*u)
		lp += 0.2 * (r.next() - lp)
		pcm[i] = (sinf(twoPi*f*vib*t)*0.5 + sinf(twoPi*2*f*vib*t)*0.2 + lp*0.55) * env
	}
	normalize(pcm, 0.85)
	return pcm
}

// synthHoof is one hoof hitting the ground: a short, dull low thud.
func synthHoof(r *noise) []float32 {
	const dur = float32(0.12)
	n := samples(dur)
	pcm := make([]float32, n)
	var lp float32
	for i := range pcm {
		t := float32(i) / sampleRate
		u := t / dur
		lp += 0.35 * (r.next() - lp)
		env := expf(-9 * u)
		pcm[i] = (sinf(twoPi*(78-20*u)*t)*0.6 + lp*1.2) * env
	}
	normalize(pcm, 0.70)
	return pcm
}

// loopify cross-fades the tail into the head so the buffer loops without a
// click.
func loopify(buf []float32, fade int) {
	n := len(buf)
	for i := 0; i < fade; i++ {
		a := float32(i) / float32(fade)
		buf[n-fade+i] = buf[n-fade+i]*(1-a) + buf[i]*a
	}
}

func normalize(pcm []float32, target float32) {
	var peak float32
	for _, v := range pcm {
		if a := float32(math.Abs(float64(v))); a > peak {
			peak = a
		}
	}
	if peak == 0 {
		return
	}
	scale := target / peak
	for i := range pcm {
		pcm[i] *= scale
	}
}

// toPCM converts the mix to the mono 16-bit samples OpenAL uploads.
func toPCM(samples []float32) []int16 {
	pcm := make([]int16, len(samples))
	for i, v := range samples {
		switch {
		case v > 1:
			v = 1
		case v < -1:
			v = -1
		}
		pcm[i] = int16(v * 32000)
	}
	return pcm
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
