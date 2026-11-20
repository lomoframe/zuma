package font

import _ "embed"

// The signboards need a CJK face at runtime. A ~15 KB glyph subset of Source Han
// Sans CN is bundled so the game stays a single executable; regenerate it with
// `make font` after changing the sign text. It is parsed straight from memory,
// so nothing has to be unpacked to disk on start up.
//
//go:embed SourceHanSansCN-Signs.otf
var embeddedFace []byte

// Embedded returns the bundled sign font data.
func Embedded() []byte { return embeddedFace }
