// Command splashart writes the embedded loading-screen art. Run it from the
// repository root after changing internal/splashart:
//
//	go run ./tools/splashart
package main

import (
	"flag"
	"image/png"
	"log"
	"os"

	"zuma/internal/splashart"
)

func main() {
	out := flag.String("out", "internal/render/splash_bg.png", "PNG to write")
	flag.Parse()

	img := splashart.Render()
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d bytes, %dx%d)", *out, info.Size(), splashart.ArtW, splashart.ArtH)
}
