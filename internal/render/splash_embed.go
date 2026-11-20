package render

import _ "embed"

// splashPNG is the painted loading art. It is regenerated with
// `make splash` and embedded so the game stays a single executable.
//
//go:embed splash_bg.png
var splashPNG []byte
