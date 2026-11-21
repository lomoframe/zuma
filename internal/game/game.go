package game

import (
	"math"
	"math/rand"

	"github.com/go-gl/mathgl/mgl32"
)

const (
	// BallR is the ball radius on the board; Diam is the spacing of touching
	// balls in the chain.
	BallR float32 = 0.062
	Diam  float32 = 2 * BallR

	shotSpeed  float32 = 14.8
	maxInAir           = 2
	suckSpeed  float32 = 4.0
	holesInset float32 = 0.0
)

// State is the round's lifecycle.
type State int

const (
	StatePlaying State = iota
	StateWon
	StateLost
)

// RGB is one palette entry, linear-ish sRGB values in 0..1.
type RGB struct{ R, G, B float32 }

// Palette holds the ball colours. They are deliberately saturated: the board is
// dark felt, so six vivid hues stay distinguishable in the headset.
var Palette = []RGB{
	{0.93, 0.20, 0.16}, // 红
	{0.98, 0.78, 0.10}, // 黄
	{0.18, 0.52, 0.97}, // 蓝
	{0.20, 0.78, 0.34}, // 绿
	{0.66, 0.30, 0.92}, // 紫
	{0.96, 0.50, 0.13}, // 橙
}

// Ball is one bead in the chain. S is its arc length along the path.
type Ball struct {
	S     float32
	Color int
	ID    int

	// InsertAnim counts down while a freshly inserted bead slides from the
	// point it struck into its slot.
	InsertAnim float32
	From       mgl32.Vec2
}

// InsertAnimTime is how long a bead takes to slide into the chain.
const InsertAnimTime float32 = 0.13

// Shot is a ball in flight: the player's controller fires it into the board,
// so it travels in three dimensions until it meets a bead.
type Shot struct {
	Pos   mgl32.Vec3
	Vel   mgl32.Vec3
	Color int
}

// Popup is a floating score label in board space.
type Popup struct {
	Pos  mgl32.Vec2
	Text string
	Life float32
	Max  float32
}

// EventKind is a gameplay moment the presentation layer can react to.
type EventKind int

const (
	EventShoot EventKind = iota
	EventInsert
	EventMatch
	EventLose
	EventWin
	EventLevel
)

// Event is one such moment; Value carries the run length or combo.
type Event struct {
	Kind  EventKind
	Value int
}

// LevelSpec is the difficulty of one round.
type LevelSpec struct {
	Balls  int
	Colors int
	Speed  float32
}

// SpecFor returns the round's parameters.
func SpecFor(level int) LevelSpec {
	if level < 1 {
		level = 1
	}
	balls := 45 + 8*(level-1)
	if balls > 90 {
		balls = 90
	}
	colors := 4 + (level-1)/2
	if colors > len(Palette) {
		colors = len(Palette)
	}
	speed := 0.20 + 0.022*float32(level-1)
	if speed > 0.52 {
		speed = 0.52
	}
	return LevelSpec{Balls: balls, Colors: colors, Speed: speed}
}

// Game is the whole round state.
type Game struct {
	Path  *Path
	Board Board

	Balls []Ball
	Shots []Shot
	Queue []int

	Speed     float32
	Level     int
	State     State
	Score     int
	Best      int
	Combo     int
	LevelSpec LevelSpec

	Loaded int
	Next   int

	Rng *rand.Rand

	// Push animation: balls from index pushFrom on are drawn push metres behind
	// their logical position while an insertion settles.
	pushFrom int
	push     float32
	// KnockVel is the chain's recoil velocity (m/s, negative = away from the
	// hole). Combos add an impulse; friction brings it back to rest.
	KnockVel float32
	// pendingKnock is a combo impulse waiting for the split chain's two halves
	// to collide before it is applied.
	pendingKnock float32

	// pendingAhead is the ID of the ball that sits just ahead of a gap left by a
	// match. When the tail catches up, the junction may pop again.
	pendingAhead int
	nextID       int

	Popups []Popup
	Events []Event

	stateT float32
	loseT  float32
	Flash  float32
}

// New starts a fresh run at level 1.
func New(seed int64) *Game {
	g := &Game{
		Path:         NewPathForBoard(),
		Rng:          rand.New(rand.NewSource(seed)),
		pushFrom:     -1,
		nextID:       1,
		pendingAhead: -1,
		Best:         0,
	}
	g.Level = 1
	g.Path = NewPathForLevel(g.Level, g.Rng)
	g.startLevel()
	return g
}

// startLevel seeds the board with a short chain and a queue of balls still to
// come in.
func (g *Game) startLevel() {
	spec := SpecFor(g.Level)
	g.Board, g.Path = NewLayoutForLevel(g.Level, g.Rng)
	g.LevelSpec = spec
	g.Speed = spec.Speed
	g.State = StatePlaying
	g.Shots = nil
	g.Popups = nil
	g.Balls = nil
	g.Combo = 0
	g.pendingAhead = -1
	g.stateT = 0
	g.loseT = 0
	g.Flash = 0
	g.KnockVel = 0
	g.spawnChain()
}

// spawnChain lays out a fresh bead chain on the current board. It is used when a
// level starts and when a full combo shoves every bead back into the hole, so
// the table never sits empty while the recoil decays.
func (g *Game) spawnChain() {
	spec := g.LevelSpec
	g.Balls = nil
	g.Combo = 0
	g.pendingAhead = -1
	g.KnockVel = 0
	g.pendingKnock = 0

	prefill := 16 + 2*(g.Level-1)
	if prefill > spec.Balls/2 {
		prefill = spec.Balls / 2
	}
	for i := 0; i < prefill; i++ {
		// Back to front, so the head ends up closest to the hole.
		g.Balls = append(g.Balls, Ball{
			S:     float32(prefill-1-i) * Diam,
			Color: g.randColor(spec.Colors),
			ID:    g.nextID,
		})
		g.nextID++
	}
	// Keep the very first chain free of accidental runs, so the opening shot is
	// a decision rather than a free pop.
	for i := 2; i < len(g.Balls); i++ {
		if g.Balls[i].Color == g.Balls[i-1].Color && g.Balls[i].Color == g.Balls[i-2].Color {
			g.Balls[i].Color = (g.Balls[i].Color + 1) % spec.Colors
		}
	}
	g.Queue = make([]int, spec.Balls-prefill)
	for i := range g.Queue {
		g.Queue[i] = g.randColor(spec.Colors)
	}
	g.Loaded = g.pickColor(spec.Colors)
	g.Next = g.pickColor(spec.Colors)
}

func (g *Game) randColor(n int) int {
	if n < 1 {
		n = 1
	}
	return g.Rng.Intn(n)
}

// allBeadsInHole reports whether the whole chain sits behind the entrance.
func (g *Game) allBeadsInHole() bool {
	if len(g.Balls) == 0 {
		return false
	}
	for _, b := range g.Balls {
		if b.S >= 0 {
			return false
		}
	}
	return true
}

// colourOnBoard reports whether a colour still has beads that can be matched:
// either on the chain or in flight.
func (g *Game) colourOnBoard(c, n int) bool {
	if c < 0 || c >= n {
		return false
	}
	for _, b := range g.Balls {
		if b.Color == c {
			return true
		}
	}
	for _, sh := range g.Shots {
		if sh.Color == c {
			return true
		}
	}
	return false
}

// pickColor chooses the colour of the bead the launcher hands the player. A
// colour that has been cleared from the board is never offered: it could not
// match anything and would only waste a shot. Every colour still on the chain
// (or in the air) is equally likely.
//
// Only the player's beads are filtered. The chain refills with randColor over
// all colours, so a cleared colour comes back onto the board - and back into
// the hand - as soon as it spawns again.
func (g *Game) pickColor(n int) int {
	if n < 1 {
		n = 1
	}
	live := 0
	for c := 0; c < n; c++ {
		if g.colourOnBoard(c, n) {
			live++
		}
	}
	if live == 0 {
		// Nothing to match yet: an empty board still needs a bead.
		return g.randColor(n)
	}
	pick := g.Rng.Intn(live)
	for c := 0; c < n; c++ {
		if !g.colourOnBoard(c, n) {
			continue
		}
		if pick == 0 {
			return c
		}
		pick--
	}
	return g.randColor(n)
}

// volleySpread is the half-angle of the celebration cone.
const volleySpread = 20.0 * math.Pi / 180.0

// DryVolley fires n ordinary beads from o in a 20° cone around d. They are
// plain shots — same speed, same arc, same rules as one the player fires —
// cycling through every palette colour so the whole set shows up.
func (g *Game) DryVolley(o, d mgl32.Vec3, n int) {
	if n <= 0 || d.Len() < 1e-4 {
		return
	}
	d = d.Normalize()
	up := mgl32.Vec3{0, 1, 0}
	if math.Abs(float64(d.Dot(up))) > 0.9 {
		up = mgl32.Vec3{1, 0, 0}
	}
	u := d.Cross(up).Normalize()
	v := d.Cross(u).Normalize()
	tan := float32(math.Tan(volleySpread))
	for i := 0; i < n; i++ {
		ang := g.Rng.Float32() * 2 * math.Pi
		rad := float32(math.Sqrt(float64(g.Rng.Float32()))) * tan
		dir := d.Add(u.Mul(rad * float32(math.Cos(float64(ang))))).
			Add(v.Mul(rad * float32(math.Sin(float64(ang))))).Normalize()
		g.Shots = append(g.Shots, Shot{
			Pos:   o.Add(dir.Mul(0.06)),
			Vel:   dir.Mul(shotSpeed),
			Color: i % len(Palette),
		})
	}
}

// Restart goes back to level 1 and clears the score.
func (g *Game) Restart() {
	g.Level = 1
	g.Score = 0
	g.startLevel()
}

// NextLevel advances after a cleared board.
func (g *Game) NextLevel() {
	if g.Score > g.Best {
		g.Best = g.Score
	}
	g.Level++
	g.Events = append(g.Events, Event{Kind: EventLevel})
	g.startLevel()
}

// Fire launches the loaded ball from the player's controller along dir. Both
// are in board space; the caller converts the headset ray with the board
// inverse transform.
func (g *Game) Fire(origin, dir mgl32.Vec3) bool {
	if g.State != StatePlaying || len(g.Shots) >= maxInAir {
		return false
	}
	if dir.Len() < 1e-4 {
		return false
	}
	dir = dir.Normalize()
	start := origin.Add(dir.Mul(0.06))
	g.Combo = 0 // a new bead starts its own combo count
	g.Shots = append(g.Shots, Shot{Pos: start, Vel: dir.Mul(shotSpeed), Color: g.Loaded})
	g.Events = append(g.Events, Event{Kind: EventShoot})
	g.Loaded = g.Next
	if !g.colourOnBoard(g.Loaded, g.LevelSpec.Colors) {
		g.Loaded = g.pickColor(g.LevelSpec.Colors)
	}
	g.Next = g.pickColor(g.LevelSpec.Colors)
	return true
}

// DemoState puts the round in a finished state so a screenshot can show the
// overlays without playing a level through.
func (g *Game) DemoState(kind string) {
	switch kind {
	case "won":
		g.Balls = nil
		g.Queue = nil
		g.Shots = nil
		g.Score = 1240
		g.State = StateWon
		g.stateT = 0
	case "lost":
		g.Balls = nil
		g.Queue = nil
		g.Shots = nil
		g.Score = 640
		g.State = StateLost
		g.loseT = 3
	}
}

// TakeEvents hands the pending presentation events to the caller and clears
// the queue.
func (g *Game) TakeEvents() []Event {
	if len(g.Events) == 0 {
		return nil
	}
	out := g.Events
	g.Events = nil
	return out
}

// PushFrom and Push describe the insertion recoil used by the renderer.
func (g *Game) PushFrom() int { return g.pushFrom }

// Push is how far the tail is drawn behind its logical position.
func (g *Game) Push() float32 { return g.push }

// LoseReady reports that the loss animation has played out and the overlay can
// offer a restart.
func (g *Game) LoseReady() bool {
	return g.State == StateLost && (len(g.Balls) == 0 || g.loseT > 2.6)
}

// BallPos is the board position of a chain bead.
func (g *Game) BallPos(b Ball) mgl32.Vec2 {
	return g.Path.PointAt(b.S)
}

// Danger is 0 far from the hole and 1 when the head is about to be swallowed.
func (g *Game) Danger() float32 {
	if len(g.Balls) == 0 {
		return 0
	}
	d := g.Path.Len - g.Balls[0].S
	if d > 1.2 {
		return 0
	}
	return 1 - d/1.2
}

// Update advances the whole round.
func (g *Game) Update(dt float32) {
	if dt > 0.1 {
		dt = 0.1
	}
	if g.Flash > 0 {
		g.Flash -= dt * 2.2
	}
	for i := range g.Balls {
		if g.Balls[i].InsertAnim > 0 {
			g.Balls[i].InsertAnim -= dt
			if g.Balls[i].InsertAnim < 0 {
				g.Balls[i].InsertAnim = 0
			}
		}
	}
	for i := range g.Popups {
		g.Popups[i].Life -= dt
	}
	keep := g.Popups[:0]
	for _, p := range g.Popups {
		if p.Life > 0 {
			keep = append(keep, p)
		}
	}
	g.Popups = keep

	switch g.State {
	case StateWon:
		// The player presses fire to move on; Update only keeps the overlay
		// and the celebration volley running.
		g.stateT += dt
		g.moveShots(dt)
		return
	case StateLost:
		g.loseT += dt
	}

	if g.push > 0 {
		g.push -= dt * 6.0
		if g.push < 0 {
			g.push = 0
			g.pushFrom = -1
		}
	}

	g.moveShots(dt)
	if g.State == StatePlaying {
		g.spawn()
	}
	g.stepChain(dt)
	// A bead in hand must stay usable: if its colour has just been cleared
	// from the board, swap it for one that still has beads to match.
	if g.State == StatePlaying && !g.colourOnBoard(g.Loaded, g.LevelSpec.Colors) {
		g.Loaded = g.pickColor(g.LevelSpec.Colors)
	}

	if g.State == StatePlaying && len(g.Balls) > 0 {
		if g.Balls[0].S >= g.Path.Len-holesInset {
			g.State = StateLost
			g.loseT = 0
			g.Events = append(g.Events, Event{Kind: EventLose})
		}
	}
	if g.State == StatePlaying && len(g.Balls) == 0 && len(g.Queue) == 0 && len(g.Shots) == 0 {
		g.State = StateWon
		g.stateT = 0
		g.Events = append(g.Events, Event{Kind: EventWin})
		g.Score += 150 * g.Level
	}
}

// spawn feeds the next queued ball in at the tail once there is room.
func (g *Game) spawn() {
	if len(g.Queue) == 0 {
		return
	}
	if len(g.Balls) > 0 && g.Balls[len(g.Balls)-1].S < Diam {
		return
	}
	c := g.Queue[0]
	g.Queue = g.Queue[1:]
	g.Balls = append(g.Balls, Ball{S: 0, Color: c, ID: g.nextID})
	g.nextID++
}

// stepChain moves the chain and closes gaps. Balls ahead of a gap catch up at
// triple speed, which is how a cleared run heals.
func (g *Game) stepChain(dt float32) {
	if len(g.Balls) == 0 {
		return
	}
	holeS := g.Path.Len

	v := g.Speed
	if g.State == StateLost {
		v = suckSpeed
	} else if g.State != StatePlaying {
		v = 0
	}

	// Knockback: the whole chain shares one recoil velocity. The tail may be
	// pushed back into the entrance hole (its beads are then inside the hole and
	// are not drawn), so a combo that lands while the chain is still being
	// released keeps shoving instead of dying at s=0.
	if g.KnockVel != 0 && g.State == StatePlaying {
		shift := g.KnockVel * dt
		if shift < 0 {
			if minShift := -knockRetreat - g.Balls[len(g.Balls)-1].S; shift < minShift {
				shift = minShift
				g.KnockVel = 0
			}
		}
		for i := range g.Balls {
			g.Balls[i].S += shift
		}
		// Every bead pushed into the hole: stop the recoil right here and lay
		// a fresh chain, instead of leaving the table empty while it decays.
		if g.allBeadsInHole() {
			g.KnockVel = 0
			g.pendingKnock = 0
			g.spawnChain()
		} else {
			g.KnockVel *= float32(math.Exp(-6.0 * float64(dt)))
			if g.KnockVel > -0.02 && g.KnockVel < 0.02 {
				g.KnockVel = 0
			}
		}
	} else if g.State != StatePlaying {
		g.KnockVel = 0
	}

	// Find the first gap in the chain.
	gap := -1
	for i := 1; i < len(g.Balls); i++ {
		if g.Balls[i-1].S-g.Balls[i].S > Diam*1.02 {
			gap = i
			break
		}
	}

	switch {
	case gap < 0 || g.State == StateLost:
		// One connected chain: everything rolls forward together.
		g.Balls[0].S += v * dt
		if g.Balls[0].S > holeS {
			g.Balls[0].S = holeS
		}
		for i := 1; i < len(g.Balls); i++ {
			ahead := g.Balls[i-1]
			cur := &g.Balls[i]
			cur.S += v * dt
			if lim := ahead.S - Diam; cur.S > lim {
				cur.S = lim
			}
			if cur.S < -knockRetreat {
				cur.S = -knockRetreat
			}
		}
	case g.pendingAhead >= 0 && g.State == StatePlaying:
		// A popped run split the chain: the front section rolls back to meet
		// the rear one (Zuma's pull-back), the rear waits instead of rushing.
		room := g.Balls[gap-1].S - g.Balls[gap].S - Diam
		step := v * 3 * dt
		if room < 0 {
			room = 0
		}
		if step > room {
			step = room
		}
		for i := 0; i < gap; i++ {
			g.Balls[i].S -= step
		}
	default:
		// A gap at the tail (a freshly spawned bead): let the rear catch up.
		for i := gap; i < len(g.Balls); i++ {
			ahead := g.Balls[i-1]
			cur := &g.Balls[i]
			cur.S += v * 3 * dt
			if lim := ahead.S - Diam; cur.S > lim {
				cur.S = lim
			}
			if cur.S < 0 {
				cur.S = 0
			}
		}
	}

	// A swallowed bead disappears into the hole.
	if g.State == StateLost {
		for len(g.Balls) > 0 && g.Balls[0].S >= holeS-0.005 {
			g.Balls = g.Balls[1:]
		}
	}

	// Chain reactions: the bead ahead of a cleared run watches its gap.
	if g.pendingAhead >= 0 && g.State == StatePlaying {
		idx := g.indexOfID(g.pendingAhead)
		if idx < 0 {
			g.pendingAhead = -1
		} else if idx+1 < len(g.Balls) {
			if g.Balls[idx].S-g.Balls[idx+1].S <= Diam*1.03 {
				g.pendingAhead = -1
				// The two halves have collided: now the combo knockback lands.
				if g.pendingKnock > 0 {
					g.KnockVel = clampKnock(g.KnockVel - g.pendingKnock)
					g.pendingKnock = 0
				}
				g.matchAt(idx)
			}
		} else {
			g.pendingAhead = -1
		}
	} else if g.pendingAhead >= 0 {
		g.pendingAhead = -1
	}
}

func (g *Game) indexOfID(id int) int {
	for i := range g.Balls {
		if g.Balls[i].ID == id {
			return i
		}
	}
	return -1
}

// moveShots flies the air balls, resolves one insertion per hit, and drops the
// ones that leave the board.
func (g *Game) moveShots(dt float32) {
	if len(g.Shots) == 0 {
		return
	}
	keep := g.Shots[:0]
	for _, sh := range g.Shots {
		p0 := sh.Pos
		p1 := p0.Add(sh.Vel.Mul(dt))
		hit, idx := g.hitTest(p0, p1)
		if idx >= 0 {
			g.insert(hit, idx, sh.Color)
			continue
		}
		// A miss stops on the table: the bead follows the ray to its end
		// instead of dropping through the board.
		if p1.Y() <= 0.03 || p1.Y() > 3.5 || (mgl32.Vec2{p1.X(), p1.Z()}).Len() > BoardRadius+1.2 {
			continue
		}
		sh.Pos = p1
		keep = append(keep, sh)
	}
	g.Shots = keep
}

// hitTest finds the first bead the shot segment passes through. Beads sit at
// ball height on the board, so the test is a 3D point-to-segment distance.
func (g *Game) hitTest(p0, p1 mgl32.Vec3) (mgl32.Vec3, int) {
	bestT := float32(math.MaxFloat32)
	bestIdx := -1
	var bestPos mgl32.Vec3
	for i, b := range g.Balls {
		q := g.Path.PointAt(b.S)
		c := mgl32.Vec3{q.X(), BallR, q.Y()}
		t, d := segPoint(p0, p1, c)
		if d <= Diam*0.92 && t < bestT {
			bestT = t
			bestIdx = i
			bestPos = c
		}
	}
	if bestIdx >= 0 {
		return bestPos, bestIdx
	}
	return mgl32.Vec3{}, -1
}

// segPoint returns the parameter of the closest point on a-b and the distance.
func segPoint(a, b, p mgl32.Vec3) (float32, float32) {
	ab := b.Sub(a)
	l2 := ab.LenSqr()
	if l2 <= 1e-12 {
		return 0, p.Sub(a).Len()
	}
	t := p.Sub(a).Dot(ab) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return t, p.Sub(a.Add(ab.Mul(t))).Len()
}

// insert pushes the shot ball into the chain at the hit bead, then resolves any
// match it made.
func (g *Game) insert(hitPos mgl32.Vec3, k int, color int) {
	if k < 0 || k >= len(g.Balls) {
		return
	}
	_, tan := g.Path.At(g.Balls[k].S)
	hit2 := mgl32.Vec2{hitPos.X(), hitPos.Z()}
	j := k
	if hit2.Sub(g.Path.PointAt(g.Balls[k].S)).Dot(tan) > 0 {
		j = k // ahead of the bead, towards the hole
	} else {
		j = k + 1
	}

	if j <= 0 {
		newS := g.Balls[0].S + Diam
		if newS > g.Path.Len-0.015 {
			j = 1
		} else {
			nb := Ball{S: newS, Color: color, ID: g.nextID, InsertAnim: InsertAnimTime, From: mgl32.Vec2{hit2.X(), hit2.Y()}}
			g.nextID++
			g.Balls = append([]Ball{nb}, g.Balls...)
			g.pushFrom, g.push = -1, 0
			g.matchAt(0)
			return
		}
	}
	if j > 0 && j <= len(g.Balls) {
		prevS := g.Balls[j-1].S
		for i := j; i < len(g.Balls); i++ {
			g.Balls[i].S -= Diam
			if g.Balls[i].S < 0 {
				g.Balls[i].S = 0
			}
		}
		nb := Ball{S: prevS - Diam, Color: color, ID: g.nextID, InsertAnim: InsertAnimTime, From: mgl32.Vec2{hit2.X(), hit2.Y()}}
		g.nextID++
		g.Balls = append(g.Balls, Ball{})
		copy(g.Balls[j+1:], g.Balls[j:])
		g.Balls[j] = nb
		g.pushFrom, g.push = j+1, Diam
		g.Events = append(g.Events, Event{Kind: EventInsert})
		g.matchAt(j)
	}
}

// matchAt removes the run of same-coloured beads around idx when it is three
// or longer and physically contiguous.
func (g *Game) matchAt(idx int) {
	if idx < 0 || idx >= len(g.Balls) {
		return
	}
	color := g.Balls[idx].Color
	lo, hi := idx, idx
	for lo > 0 && g.Balls[lo-1].Color == color && g.Balls[lo-1].S-g.Balls[lo].S <= Diam*1.3 {
		lo--
	}
	for hi+1 < len(g.Balls) && g.Balls[hi+1].Color == color && g.Balls[hi].S-g.Balls[hi+1].S <= Diam*1.3 {
		hi++
	}
	n := hi - lo + 1
	if n < 3 {
		return
	}
	g.Combo++
	mult := g.Combo
	if mult > 9 {
		mult = 9
	}
	gain := 10 * n * mult
	g.Score += gain
	g.Events = append(g.Events, Event{Kind: EventMatch, Value: mult})
	g.Flash = 1
	mid := g.Path.PointAt((g.Balls[lo].S + g.Balls[hi].S) / 2)
	g.Popups = append(g.Popups, Popup{
		Pos:  mid,
		Text: "+" + itoa(gain),
		Life: 1.1,
		Max:  1.1,
	})
	if g.Score > g.Best {
		g.Best = g.Score
	}

	// Remember the bead ahead of the gap for the chain reaction. Indices after
	// the removal shift, so hand over an ID instead.
	aheadID := -1
	if lo > 0 {
		aheadID = g.Balls[lo-1].ID
	}
	copy(g.Balls[lo:], g.Balls[hi+1:])
	g.Balls = g.Balls[:len(g.Balls)-(hi-lo+1)]
	// The combo impulse is held until the front and rear halves meet again;
	// if the pop touched an end of the chain there is no collision to wait for.
	if impulse := comboImpulse(g.Combo); impulse > 0 {
		if lo > 0 && lo < len(g.Balls) {
			g.pendingKnock += impulse
		} else {
			g.KnockVel = clampKnock(g.KnockVel - impulse)
		}
	}
	if lo > 0 && lo < len(g.Balls) && aheadID >= 0 {
		g.pendingAhead = aheadID
	} else {
		g.pendingAhead = -1
	}
	g.pushFrom, g.push = -1, 0
}

// knockMax bounds the chain recoil. The impulse grows with every extra pop, so
// it needs a ceiling that keeps the beads inside the groove.
const knockMax float32 = 9

// knockRetreat is how far past the entrance the chain's tail may be pushed: a
// few beads vanish into the hole, which is what a combo landing while the chain
// is still coming out should look like.
const knockRetreat float32 = 0.9

// comboImpulse is the recoil (m/s, away from the hole) for c pops caused by one
// shot: nothing on the first pop, then a strictly stronger push for every level.
// The tanh keeps that growth visible at each step without letting a long combo
// fire the chain off the board.
func comboImpulse(c int) float32 {
	if c < 2 {
		return 0
	}
	raw := 0.8*float32(c) + 0.35*float32(c)*float32(c)
	return knockMax * float32(math.Tanh(float64(raw/knockMax)))
}

// clampKnock keeps a recoil velocity inside the range the chain can absorb.
func clampKnock(v float32) float32 {
	if v < -knockMax {
		return -knockMax
	}
	return v
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
