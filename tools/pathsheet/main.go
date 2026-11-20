// Command pathsheet renders candidate board routes as numbered top-down PNGs so
// a layout can be picked by eye before it is wired into the level generator.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"zuma/internal/game"
)

type candidate struct {
	name  string
	board game.Board
	path  *game.Path
}

var digits = [10][7]string{
	{"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	{"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	{"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	{"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	{"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	{"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	{"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	{"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	{"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	{"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
}

func main() {
	out := flag.String("out", "design/routes", "output directory")
	size := flag.Int("size", 760, "canvas size of each sheet cell")
	samples := flag.Int("samples", 0, "render N real random layouts instead of the catalog")
	doodles := flag.Bool("doodles", false, "render one sample per one-stroke silhouette")
	flag.Parse()

	cands := candidates()
	if *samples > 0 {
		cands = sampleLayouts(*samples)
	}
	if *doodles {
		cands = doodleSheet()
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		panic(err)
	}
	cells := make([]*image.RGBA, len(cands))
	for i, c := range cands {
		img := render(c.board, c.path, *size, i+1)
		cells[i] = img
		f, err := os.Create(filepath.Join(*out, fmt.Sprintf("%02d.png", i+1)))
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		fmt.Printf("%02d  %-34s  len %.1f m\n", i+1, c.name, c.path.Len)
	}
	cols := 4
	if len(cells) <= 6 {
		cols = 3
	}
	sheet := contact(cells, cols, 10)
	name := "contact.png"
	if *samples > 0 {
		name = "samples.png"
	}
	if *doodles {
		name = "doodles.png"
	}
	f, err := os.Create(filepath.Join(*out, name))
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, sheet); err != nil {
		panic(err)
	}
	f.Close()
	fmt.Println("wrote", filepath.Join(*out, "contact.png"))
}

// sampleLayouts renders what the game will actually deal out: board + route from
// NewLayoutForLevel for consecutive seeds.
func sampleLayouts(n int) []candidate {
	out := make([]candidate, 0, n)
	for seed := int64(1); seed <= int64(n); seed++ {
		b, p := game.NewLayoutForLevel(1, rand.New(rand.NewSource(seed)))
		out = append(out, candidate{fmt.Sprintf("seed %d · %s", seed, strings.SplitN(p.Key, " ", 2)[0]), b, p})
	}
	return out
}

// doodleSheet shows one sample per one-stroke silhouette, so the shapes can be
// checked on their own instead of being diluted by the other families.
func doodleSheet() []candidate {
	b := game.CircleBoard(4.95) // outlines get the grown board, same as in game
	names := []struct {
		label string
		key   string
	}{
		{"心形", "heart"}, {"五角星", "star"}, {"苹果", "apple"},
		{"五角形", "penta"}, {"眼睛", "eye"}, {"水杯", "cup"},
		{"香蕉", "banana"}, {"手柄", "pad"},
	}
	out := make([]candidate, 0, len(names))
	for _, n := range names {
		outline, rounds, ok := game.Doodle(n.key)
		if !ok {
			continue
		}
		out = append(out, candidate{n.label, b, game.NewDoodleOutline(b, 0.18, n.key, outline, rounds)})
	}
	return out
}

func candidates() []candidate {
	var out []candidate
	for _, c := range game.CatalogRoutes() {
		out = append(out, candidate{c.Name, c.Board, c.Path})
	}
	return out
}

var (
	bgCol    = color.RGBA{16, 17, 21, 255}
	woodCol  = color.RGBA{92, 62, 44, 255}
	feltCol  = color.RGBA{30, 84, 86, 255}
	grooveBg = color.RGBA{9, 20, 24, 255}
	routeCol = color.RGBA{120, 214, 210, 255}
	entryCol = color.RGBA{96, 224, 120, 255}
	holeCol  = color.RGBA{236, 88, 72, 255}
	inkCol   = color.RGBA{236, 240, 245, 255}
)

func render(b game.Board, p *game.Path, size, number int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	fill(img, bgCol)

	const view = 2.10 // world half-extent shown, shared so boards compare by size
	scale := float64(size) / (2 * view)
	cx, cy := float64(size)/2, float64(size)/2

	shape(img, b.Inflate(0.13), cx, cy, scale, woodCol)
	shape(img, b, cx, cy, scale, feltCol)
	rim(img, b, cx, cy, scale)

	strokeWidth := math.Max(5, 0.150*scale) // the channel
	innerWidth := math.Max(2, 0.055*scale)  // the bright route line
	for s := float32(0); s <= p.Len; s += 0.008 {
		pt := p.PointAt(s)
		x := cx + float64(pt.X())*scale
		y := cy + float64(pt.Y())*scale
		disc(img, x, y, strokeWidth/2, grooveBg)
		disc(img, x, y, innerWidth/2, routeCol)
	}

	startP := p.PointAt(0)
	endP := p.PointAt(p.Len)
	disc(img, cx+float64(startP.X())*scale, cy+float64(startP.Y())*scale, math.Max(9, 0.11*scale), entryCol)
	disc(img, cx+float64(endP.X())*scale, cy+float64(endP.Y())*scale, math.Max(12, 0.15*scale), grooveBg)
	ring(img, cx+float64(endP.X())*scale, cy+float64(endP.Y())*scale, math.Max(12, 0.15*scale), holeCol)

	drawNumber(img, size/28, size/28, number, size/13, inkCol)
	return img
}

// shape fills a board silhouette pixel by pixel from its half-width profile.
func shape(img *image.RGBA, b game.Board, cx, cy, scale float64, c color.RGBA) {
	half := float64(b.HalfH()) * scale
	y0, y1 := int(cy-half)-1, int(cy+half)+1
	for y := y0; y <= y1; y++ {
		z := float32((float64(y) - cy) / scale)
		w := float64(b.WidthAt(z)) * scale
		if w <= 0 {
			continue
		}
		xa, xb := int(cx-w)-1, int(cx+w)+1
		for x := xa; x <= xb; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// rim draws a thin copper line along the felt edge.
func rim(img *image.RGBA, b game.Board, cx, cy, scale float64) {
	for _, pt := range b.Outline(220) {
		x := cx + float64(pt.X())*scale
		y := cy + float64(pt.Y())*scale
		disc(img, x, y, 2.2, color.RGBA{150, 104, 52, 255})
	}
}

func contact(cells []*image.RGBA, cols, margin int) *image.RGBA {
	rows := (len(cells) + cols - 1) / cols
	w := cells[0].Bounds().Dx()
	sheet := image.NewRGBA(image.Rect(0, 0, cols*w+(cols+1)*margin, rows*w+(rows+1)*margin))
	fill(sheet, color.RGBA{8, 9, 11, 255})
	for i, c := range cells {
		ox := margin + (i%cols)*(w+margin)
		oy := margin + (i/cols)*(w+margin)
		for y := 0; y < w; y++ {
			for x := 0; x < w; x++ {
				sheet.Set(ox+x, oy+y, c.At(x, y))
			}
		}
	}
	return sheet
}

func fill(img *image.RGBA, c color.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func disc(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	x0, x1 := int(cx-r-1), int(cx+r+1)
	y0, y1 := int(cy-r-1), int(cy+r+1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func ring(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	x0, x1 := int(cx-r-3), int(cx+r+3)
	y0, y1 := int(cy-r-3), int(cy+r+3)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d := math.Sqrt(dx*dx + dy*dy)
			if d >= r-2.5 && d <= r+2.5 {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func drawNumber(img *image.RGBA, x, y, n, h int, c color.RGBA) {
	w := h * 5 / 7
	text := fmt.Sprintf("%d", n)
	for i, ch := range text {
		d := digits[ch-'0']
		ox := x + i*(w+h/7)
		for row := 0; row < 7; row++ {
			for col := 0; col < 5; col++ {
				if d[row][col] != '1' {
					continue
				}
				for yy := 0; yy < h/7; yy++ {
					for xx := 0; xx < w/5; xx++ {
						img.SetRGBA(ox+col*(w/5)+xx, y+row*(h/7)+yy, c)
					}
				}
			}
		}
	}
}
