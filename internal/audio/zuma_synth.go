package audio

// Zuma's own clips. Everything is a small tonal gesture: the frog's shot is a
// short pitched thock, a match is a bright marimba double, and combos climb.

// addNote mixes one plucked tone into buf at start seconds.
func addNote(buf []float32, start, dur, freq, gain float32) {
	n := samples(dur)
	off := samples(start)
	for i := 0; i < n && off+i < len(buf); i++ {
		t := float32(i) / sampleRate
		env := expf(-6.0 * t)
		attack := t / 0.006
		if attack < 1 {
			env *= attack
		}
		s := sinf(twoPi*freq*t) + 0.32*sinf(twoPi*2*freq*t) + 0.12*sinf(twoPi*3*freq*t)
		buf[off+i] += gain * env * s
	}
}

func synthShoot() []float32 {
	const dur = float32(0.16)
	pcm := make([]float32, samples(dur))
	for i := range pcm {
		t := float32(i) / sampleRate
		f := 780 - 430*(t/dur)
		body := sinf(twoPi*f*t) * expf(-24*t)
		click := sinf(twoPi*1500*t) * expf(-90*t)
		pcm[i] = 0.8*body + 0.25*click
	}
	return pcm
}

func synthInsert() []float32 {
	const dur = float32(0.10)
	pcm := make([]float32, samples(dur))
	for i := range pcm {
		t := float32(i) / sampleRate
		pcm[i] = 0.55 * sinf(twoPi*520*t) * expf(-34*t)
	}
	return pcm
}

func synthPop() []float32 {
	const dur = float32(0.26)
	pcm := make([]float32, samples(dur))
	addNote(pcm, 0, dur, 660, 0.55)
	addNote(pcm, 0.02, dur, 990, 0.32)
	return pcm
}

func synthCombo() []float32 {
	const dur = float32(0.55)
	pcm := make([]float32, samples(dur))
	notes := []float32{523.25, 659.25, 783.99, 1046.5}
	for i, f := range notes {
		addNote(pcm, 0.09*float32(i), 0.30, f, 0.42)
	}
	return pcm
}

func synthLose() []float32 {
	const dur = float32(1.10)
	pcm := make([]float32, samples(dur))
	notes := []float32{392.0, 329.63, 261.63, 196.0}
	for i, f := range notes {
		addNote(pcm, 0.20*float32(i), 0.55, f, 0.50)
	}
	return pcm
}

func synthWin() []float32 {
	const dur = float32(1.7)
	pcm := make([]float32, samples(dur))
	notes := []float32{523.25, 659.25, 783.99, 1046.5, 1318.5}
	for i, f := range notes {
		g := float32(0.42)
		if i == len(notes)-1 {
			g = 0.55
		}
		addNote(pcm, 0.13*float32(i), 0.70, f, g)
	}
	return pcm
}
