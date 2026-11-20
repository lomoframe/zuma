package font

import "testing"

// TestEmbeddedFontRenders checks the bundled subset parses and produces ink for
// the text the HUD and the overlays actually use.
func TestEmbeddedFontRenders(t *testing.T) {
	for _, text := range []string{
		"分数 1240   第 3 关", "左键 / 扳机 发射", "连击 x3   剩余 20",
		"游戏结束", "第 2 关 通过!", "按 R 或扳机 重新开始", "得分 900   最高 3200",
	} {
		line, err := Render(Embedded(), text, 24)
		if err != nil {
			t.Fatalf("Render(%q): %v", text, err)
		}
		if line.W <= 0 || line.H <= 0 {
			t.Fatalf("%q: empty mask %dx%d", text, line.W, line.H)
		}
		lit := 0
		for _, v := range line.Mask {
			if v > 32 {
				lit++
			}
		}
		if lit < 20 {
			t.Fatalf("%q: mask has almost no ink: %d lit pixels", text, lit)
		}
	}
}

// TestEmbeddedFontCoversEveryHUDGlyph checks each character on its own: a
// missing glyph is silently blank, while a line-level check can still pass
// because the other characters supply ink.
func TestEmbeddedFontCoversEveryHUDGlyph(t *testing.T) {
	for _, r := range "祖玛分数关卡第通过游戏结束胜利重新开始连击最高下发射瞄准继续并命中剩余暂停点击扳机左键球速新纪录危险快吃掉了洞口0123456789xR/+-!" {
		line, err := Render(Embedded(), string(r), 32)
		if err != nil {
			t.Fatalf("Render(%q): %v", r, err)
		}
		lit := 0
		for _, v := range line.Mask {
			if v > 32 {
				lit++
			}
		}
		if lit < 3 {
			t.Fatalf("glyph %q is missing from the embedded subset (only %d lit pixels)", r, lit)
		}
	}
}

// TestRenderSize pins the line metrics the HUD scales its quads from.
func TestRenderSize(t *testing.T) {
	line, err := Render(Embedded(), "分数", 24)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if line.W != 48 || line.H < 28 || line.H > 40 {
		t.Fatalf("size %dx%d, want 48x~31", line.W, line.H)
	}
}
