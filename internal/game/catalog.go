package game

// Candidate is one numbered route idea on its own board, shared by the design
// sheet renderer and the spacing tests so both always agree.
type Candidate struct {
	Name  string
	Board Board
	Path  *Path
}

// CatalogRoutes lists the route families that are on the table.
func CatalogRoutes() []Candidate {
	circle := CircleBoard(3.60)
	oval := Board{W: 4.10, H: 2.90, Oval: true}
	rect := Board{W: 3.60, H: 2.70, Corner: 0.70}
	long := Board{W: 4.20, H: 2.20, Corner: 0.60}
	big := CircleBoard(4.95) // one-stroke outlines get the grown board

	return []Candidate{
		// Family 1: one line winding inwards. Petal / snail / eccentric / pitch
		// are all parameter choices inside this family, so one sample is enough.
		{"单线螺旋", circle, NewPetalSpiralOnBoard(circle, 0.18, 3.2, 0.28, 5, 0.07)},
		{"异形环层（2 层）", circle, NewLayeredRingsOnBoard(circle, 0.18,
			[]RingShape{{}, {Square: 0.75, Rot: 0.5}}, []float32{1.4})},
		{"迷宫发卡", rect, NewMazePath(rect, 0.16, 5, 0.32, 0.14)},
		{"波浪发卡", long, NewWavyLanePath(long, 0.16, 5, 0.32, 0.24, 2)},
		{"方形螺旋", rect, NewSquareSpiralOnBoard(rect, 0.16, 0.32, 0.15)},
		{"圆弧发卡", oval, NewArcSwitchbackOnBoard(oval, 0.16, 6, 0.30, 2.0)},
		{"一笔画 · 五角星", big, NewDoodleOutline(big, 0.18, "star", starOutline(), 1)},
		{"一笔画 · 苹果", big, NewDoodleOutline(big, 0.18, "apple", appleOutline(), 2)},
		{"一笔画 · 五角形", big, NewDoodleOutline(big, 0.18, "penta", pentagonOutline(), 1)},
		{"一笔画 · 眼睛", big, NewDoodleOutline(big, 0.18, "eye", eyeOutline(), 2)},
		{"一笔画 · 水杯", big, NewDoodleOutline(big, 0.18, "cup", cupOutline(), 1)},
	}
}
