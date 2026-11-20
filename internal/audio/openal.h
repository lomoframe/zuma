#ifndef GOVR_AUDIO_H
#define GOVR_AUDIO_H

#ifdef __cplusplus
extern "C" {
#endif

/* Opens the default OpenAL device and creates the sources. The clips
 * themselves are synthesised in Go and handed over with govr_audio_buffer.
 * Returns 1 on success and leaves a reason in err otherwise. */
int govr_audio_open(char *err, int errlen);
void govr_audio_close(void);

/* Uploads mono 16-bit PCM at the given sample rate. Returns the OpenAL buffer
 * name, or 0 on failure. */
unsigned int govr_audio_buffer(const short *pcm, int n, int rate);

/* Sets the buffer played by the looping wind source. */
void govr_audio_wind_buffer(unsigned int buffer);

/* Plays a one-shot buffer at the given gain and pitch. */
void govr_audio_play(unsigned int buffer, float gain, float pitch);

/* Plays a one-shot buffer on the dedicated source that long clips such as the
 * celebration fanfare use. */
void govr_audio_play_solo(unsigned int buffer, float gain);

/* Drives the looping wind source: gain 0 silences it. pitch scales the loop. */
void govr_audio_wind(float gain, float pitch);

/* Sets the buffer played by the looping engine / water sources. */
void govr_audio_engine_buffer(unsigned int buffer);
void govr_audio_water_buffer(unsigned int buffer);

/* Drives the looping engine and water sources: gain 0 silences them. */
void govr_audio_engine(float gain, float pitch);
void govr_audio_water(float gain, float pitch);

#ifdef __cplusplus
}
#endif

#endif
