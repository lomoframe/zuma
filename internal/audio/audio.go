// Package audio plays the game's procedurally synthesised sounds: fireworks,
// landing thuds, boost whooshes, a looping glide wind and the fanfare that
// celebrates clearing a course.
package audio

/*
#cgo LDFLAGS: -lopenal
#include <stdlib.h>
#include "openal.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Clip identifies a one-shot sound.
type Clip int

const (
	Firework Clip = iota
	Landing
	Boost
	Cheer
	Accel
	Brake
	Footstep
	// Engine and Water are loops driven with SetEngineLoop / SetWaterLoop.
	Engine
	Water
	Splash
	Crash
	Startle
	Ignition
	Scream
	Neigh
	Hoof
	// Zuma clips.
	Shoot
	Insert
	Pop
	Combo
	Lose
	Win
)

const clipCount = 22

var (
	ready   bool
	buffers [clipCount]C.uint
	// rng is shared by the whole synthesis pass and then keeps driving the
	// per-shot pitch variation, exactly like the single seed the C version had.
	rng = &noise{state: 22222}
)

// Init opens the audio device and synthesises every clip. It is best effort:
// on failure the game keeps running silently.
func Init() error {
	buf := C.malloc(256)
	defer C.free(buf)
	if C.govr_audio_open((*C.char)(buf), 256) != 1 {
		return fmt.Errorf("%s", C.GoString((*C.char)(buf)))
	}

	// Same order as the clips have always been built, so the noise sequences
	// stay stable.
	buffers[Firework] = upload(synthFirework(rng))
	buffers[Landing] = upload(synthLanding(rng))
	buffers[Boost] = upload(synthBoost(rng))
	wind := upload(synthWind(rng))
	buffers[Cheer] = upload(synthCheer())
	buffers[Accel] = upload(synthAccel(rng))
	buffers[Brake] = upload(synthBrake(rng))
	buffers[Footstep] = upload(synthFootstep(rng))
	engine := upload(synthEngine(rng))
	water := upload(synthWater(rng))
	buffers[Engine] = engine
	buffers[Water] = water
	buffers[Splash] = upload(synthSplash(rng))
	buffers[Crash] = upload(synthCrash(rng))
	buffers[Startle] = upload(synthStartle(rng))
	buffers[Ignition] = upload(synthIgnition(rng))
	buffers[Scream] = upload(synthScream(rng))
	buffers[Neigh] = upload(synthNeigh(rng))
	buffers[Hoof] = upload(synthHoof(rng))
	buffers[Shoot] = upload(synthShoot())
	buffers[Insert] = upload(synthInsert())
	buffers[Pop] = upload(synthPop())
	buffers[Combo] = upload(synthCombo())
	buffers[Lose] = upload(synthLose())
	buffers[Win] = upload(synthWin())

	C.govr_audio_wind_buffer(wind)
	C.govr_audio_wind(0, 1)
	C.govr_audio_engine_buffer(engine)
	C.govr_audio_engine(0, 1)
	C.govr_audio_water_buffer(water)
	C.govr_audio_water(0, 1)
	ready = true
	return nil
}

func upload(samples []float32) C.uint {
	pcm := toPCM(samples)
	if len(pcm) == 0 {
		return 0
	}
	return C.govr_audio_buffer((*C.short)(unsafe.Pointer(&pcm[0])), C.int(len(pcm)), sampleRate)
}

// Close releases the device and every buffer.
func Close() {
	if ready {
		C.govr_audio_close()
		ready = false
	}
}

// Play starts a one-shot clip; gain above 1 is clamped by the backend.
func Play(clip Clip, gain float32) {
	if !ready || gain <= 0 || clip < 0 || clip >= clipCount {
		return
	}
	buf := buffers[clip]
	if buf == 0 {
		return
	}
	if clip == Cheer {
		C.govr_audio_play_solo(buf, C.float(gain))
		return
	}
	pitch := float32(1)
	switch clip {
	case Firework:
		// Vary the pitch of bursts a little so a long show does not loop
		// verbatim.
		pitch = 0.9 + 0.2*(0.5+0.5*rng.next())
	case Footstep:
		pitch = 0.92 + 0.14*(0.5+0.5*rng.next())
	case Startle:
		// A herd does not bleat in unison.
		pitch = 0.82 + 0.36*(0.5+0.5*rng.next())
	case Splash:
		pitch = 0.85 + 0.3*(0.5+0.5*rng.next())
	case Scream:
		pitch = 0.9 + 0.2*(0.5+0.5*rng.next())
	case Hoof:
		pitch = 0.9 + 0.2*(0.5+0.5*rng.next())
	}
	C.govr_audio_play(buf, C.float(gain), C.float(pitch))
}

// PlayPitched plays a one-shot at an explicit pitch, used to make combo hits
// climb with the combo level.
func PlayPitched(clip Clip, gain, pitch float32) {
	if !ready || gain <= 0 || clip < 0 || clip >= clipCount {
		return
	}
	buf := buffers[clip]
	if buf == 0 {
		return
	}
	C.govr_audio_play(buf, C.float(gain), C.float(pitch))
}

// SetEngineLoop drives the motorcycle engine loop: gain 0 silences it, pitch
// follows the revs.
func SetEngineLoop(gain, pitch float32) {
	if !ready {
		return
	}
	C.govr_audio_engine(C.float(clamp(gain, 0, 1)), C.float(clamp(pitch, 0.5, 2)))
}

// SetWaterLoop drives the wading/splash loop used while crossing the lake.
func SetWaterLoop(gain, pitch float32) {
	if !ready {
		return
	}
	C.govr_audio_water(C.float(clamp(gain, 0, 1)), C.float(clamp(pitch, 0.5, 2)))
}

// Wind drives the looping wind loop: gain 0 is silent, pitch scales its speed.
func Wind(gain, pitch float32) {
	if !ready {
		return
	}
	C.govr_audio_wind(C.float(clamp(gain, 0, 1)), C.float(clamp(pitch, 0.5, 2)))
}
