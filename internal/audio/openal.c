// Thin OpenAL backend. The clips are synthesised in Go; this file only owns
// the device, the uploaded buffers and the sources that play them.
#include "openal.h"

#include <AL/al.h>
#include <AL/alc.h>

#include <stdio.h>

#define MAX_BUFFERS 20
#define ONESHOT_SOURCES 8

static ALCdevice *g_dev;
static ALCcontext *g_ctx;
static ALuint g_buffers[MAX_BUFFERS];
static int g_buffer_count;
static ALuint g_oneshot[ONESHOT_SOURCES];
static int g_next_oneshot;
static ALuint g_wind;
static ALuint g_solo;
static ALuint g_engine;
static ALuint g_water;

static void set_err(char *err, int errlen, const char *msg) {
    if (err != NULL && errlen > 0) {
        snprintf(err, (size_t)errlen, "%s", msg);
    }
}

int govr_audio_open(char *err, int errlen) {
    if (err != NULL && errlen > 0) {
        err[0] = '\0';
    }
    g_dev = alcOpenDevice(NULL);
    if (g_dev == NULL) {
        set_err(err, errlen, "no audio device");
        return 0;
    }
    g_ctx = alcCreateContext(g_dev, NULL);
    if (g_ctx == NULL || alcMakeContextCurrent(g_ctx) == ALC_FALSE) {
        set_err(err, errlen, "no audio context");
        return 0;
    }

    alGenSources(ONESHOT_SOURCES, g_oneshot);
    for (int i = 0; i < ONESHOT_SOURCES; i++) {
        alSourcei(g_oneshot[i], AL_SOURCE_RELATIVE, AL_TRUE);
    }
    alGenSources(1, &g_solo);
    alSourcei(g_solo, AL_SOURCE_RELATIVE, AL_TRUE);
    alGenSources(1, &g_wind);
    alSourcei(g_wind, AL_SOURCE_RELATIVE, AL_TRUE);
    alSourcei(g_wind, AL_LOOPING, AL_TRUE);
    alSourcef(g_wind, AL_GAIN, 0.0f);
    alGenSources(1, &g_engine);
    alSourcei(g_engine, AL_SOURCE_RELATIVE, AL_TRUE);
    alSourcei(g_engine, AL_LOOPING, AL_TRUE);
    alSourcef(g_engine, AL_GAIN, 0.0f);
    alGenSources(1, &g_water);
    alSourcei(g_water, AL_SOURCE_RELATIVE, AL_TRUE);
    alSourcei(g_water, AL_LOOPING, AL_TRUE);
    alSourcef(g_water, AL_GAIN, 0.0f);
    return 1;
}

void govr_audio_close(void) {
    if (g_ctx == NULL) {
        return;
    }
    alSourceStopv(ONESHOT_SOURCES, g_oneshot);
    alDeleteSources(ONESHOT_SOURCES, g_oneshot);
    alSourceStop(g_wind);
    alDeleteSources(1, &g_wind);
    alSourceStop(g_solo);
    alDeleteSources(1, &g_solo);
    alSourceStop(g_engine);
    alDeleteSources(1, &g_engine);
    alSourceStop(g_water);
    alDeleteSources(1, &g_water);
    if (g_buffer_count > 0) {
        alDeleteBuffers(g_buffer_count, g_buffers);
    }
    g_buffer_count = 0;
    alcMakeContextCurrent(NULL);
    alcDestroyContext(g_ctx);
    alcCloseDevice(g_dev);
    g_ctx = NULL;
    g_dev = NULL;
}

unsigned int govr_audio_buffer(const short *pcm, int n, int rate) {
    if (g_ctx == NULL || pcm == NULL || n <= 0 || g_buffer_count >= MAX_BUFFERS) {
        return 0;
    }
    ALuint buf = 0;
    alGenBuffers(1, &buf);
    if (buf == 0) {
        return 0;
    }
    alBufferData(buf, AL_FORMAT_MONO16, pcm, (ALsizei)(n * (int)sizeof(short)), rate);
    g_buffers[g_buffer_count++] = buf;
    return buf;
}

void govr_audio_wind_buffer(unsigned int buffer) {
    if (g_ctx == NULL) {
        return;
    }
    alSourcei(g_wind, AL_BUFFER, (ALint)buffer);
}

void govr_audio_play(unsigned int buffer, float gain, float pitch) {
    if (g_ctx == NULL || buffer == 0) {
        return;
    }
    ALuint src = g_oneshot[g_next_oneshot];
    g_next_oneshot = (g_next_oneshot + 1) % ONESHOT_SOURCES;
    alSourceStop(src);
    alSourcei(src, AL_BUFFER, (ALint)buffer);
    alSourcef(src, AL_GAIN, gain);
    alSourcef(src, AL_PITCH, pitch);
    alSourcePlay(src);
}

void govr_audio_play_solo(unsigned int buffer, float gain) {
    if (g_ctx == NULL || buffer == 0) {
        return;
    }
    alSourceStop(g_solo);
    alSourcei(g_solo, AL_BUFFER, (ALint)buffer);
    alSourcef(g_solo, AL_GAIN, gain);
    alSourcePlay(g_solo);
}

void govr_audio_wind(float gain, float pitch) {
    if (g_ctx == NULL) {
        return;
    }
    alSourcef(g_wind, AL_GAIN, gain);
    alSourcef(g_wind, AL_PITCH, pitch);
    // Restart the loop if it somehow stopped (device underrun).
    ALint state = 0;
    alGetSourcei(g_wind, AL_SOURCE_STATE, &state);
    if (state != AL_PLAYING) {
        alSourcePlay(g_wind);
    }
}

void govr_audio_engine_buffer(unsigned int buffer) {
    if (g_ctx == NULL) {
        return;
    }
    alSourcei(g_engine, AL_BUFFER, (ALint)buffer);
}

void govr_audio_water_buffer(unsigned int buffer) {
    if (g_ctx == NULL) {
        return;
    }
    alSourcei(g_water, AL_BUFFER, (ALint)buffer);
}

/* Drives one of the extra loops (engine, water). gain 0 silences it. */
static void set_loop(ALuint src, float gain, float pitch) {
    alSourcef(src, AL_GAIN, gain);
    alSourcef(src, AL_PITCH, pitch);
    ALint state = 0;
    alGetSourcei(src, AL_SOURCE_STATE, &state);
    if (state != AL_PLAYING) {
        alSourcePlay(src);
    }
}

void govr_audio_engine(float gain, float pitch) {
    if (g_ctx == NULL) {
        return;
    }
    set_loop(g_engine, gain, pitch);
}

void govr_audio_water(float gain, float pitch) {
    if (g_ctx == NULL) {
        return;
    }
    set_loop(g_water, gain, pitch);
}
