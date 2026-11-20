package game

import (
	"math"
	"math/rand"
	"strings"
	"testing"
)

// minNonAdjacentGap samples a path and reports the closest two points that are
// far apart along the route, which is where separate laps would collide.
func minNonAdjacentGap(p *Path) (float32, float32, float32) {
	const ds = 0.02
	best := float32(99)
	var ax, ay float32
	var bestS [2]float32
	type sample struct{ x, y, s float32 }
	var q []sample
	for s := float32(0); s <= p.Len; s += ds {
		pt := p.PointAt(s)
		q = append(q, sample{pt.X(), pt.Y(), s})
	}
	for i := range q {
		for j := i + 1; j < len(q); j++ {
			if q[j].s-q[i].s < 0.5 {
				continue
			}
			d := float32(math.Hypot(float64(q[i].x-q[j].x), float64(q[i].y-q[j].y)))
			if d < best {
				best, ax, ay = d, q[i].x, q[i].y
				bestS = [2]float32{q[i].s, q[j].s}
			}
		}
	}
	lastPair = bestS
	return best, ax, ay
}

var lastPair [2]float32

func TestRouteFamiliesKeepBeadsApart(t *testing.T) {
	for _, c := range CatalogRoutes() {
		gap, x, y := minNonAdjacentGap(c.Path)
		t.Logf("%-38s len %5.1f m  closest %.3f m at (%.2f,%.2f) s=%.2f/%.2f", c.Name, c.Path.Len, gap, x, y, lastPair[0], lastPair[1])
		if gap < 0.135 {
			t.Errorf("%s: laps collide (%.3f m)", c.Name, gap)
		}
	}
}

func TestRandomLayoutsArePlayable(t *testing.T) {
	families := map[string]int{}
	boards := map[bool]int{}
	for seed := int64(1); seed <= 120; seed++ {
		rng := rand.New(rand.NewSource(seed))
		b, p := NewLayoutForLevel(1, rng)
		if err := ValidateRoute(b, p); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if p.Len < 7.0 {
			t.Fatalf("seed %d: short route %.1f m", seed, p.Len)
		}
		fam := strings.SplitN(p.Key, " ", 2)[0]
		families[fam]++
		boards[b.Oval] = boards[b.Oval] + 1
	}
	t.Logf("families: %v", families)
	if len(families) < 5 {
		t.Fatalf("too few families in 120 levels: %v", families)
	}
}
