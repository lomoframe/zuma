BIN := bin/zuma

# Same performance-neutral size trim govr uses: default -O2 and inlining, no C
# debug info, dead C sections discarded, stripped binary.
SIZE_CFLAGS := -O2 -g0 -ffunction-sections -fdata-sections
SIZE_GOFLAGS := -trimpath -buildvcs=false
SIZE_LDFLAGS := -s -w -buildid= -extldflags=-Wl,--gc-sections

.PHONY: build run desktop shot win lose frame test vet font clean

build:
	CGO_CFLAGS="$(SIZE_CFLAGS)" CGO_CXXFLAGS="$(SIZE_CFLAGS)" \
	CGO_LDFLAGS="-Wl,--gc-sections -Wl,--as-needed" \
	go build $(SIZE_GOFLAGS) -ldflags "$(SIZE_LDFLAGS)" -o $(BIN) ./cmd/zuma

run: build
	./$(BIN)

desktop: build
	./$(BIN) -desktop

# Screenshots run on a fixed clock, so the same flags always produce the same
# frame. -fire=false keeps the shot in flight out of the reference images.
shot: build
	./$(BIN) -desktop -screenshot docs/hero.png -frames 1600 -fire=false -aim-x 0.15 -aim-y 1.3

win: build
	./$(BIN) -desktop -screenshot docs/win.png -frames 30 -demo won

lose: build
	./$(BIN) -desktop -screenshot docs/lose.png -frames 30 -demo lost

# Steam Frame (ARM64 SteamOS). Needs an aarch64 toolchain and a user sysroot
# holding libGL/libGLEW/libopenvr_api/libopenal plus the X11 libs glfw links
# against under usr/ (see govr's README for how the sysroot was built).
CC_ARM64 ?= aarch64-linux-gnu-gcc
CXX_ARM64 ?= aarch64-linux-gnu-g++
SYS_ARM64 ?= $(HOME)/.local/aarch64-sysroot

frame:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=$(CC_ARM64) CXX=$(CXX_ARM64) \
		./scripts/frame-env.sh "$(SYS_ARM64)" \
		go build $(SIZE_GOFLAGS) -ldflags "$(SIZE_LDFLAGS)" -o bin/zuma-frame ./cmd/zuma

test:
	go test ./...

vet:
	go vet ./...

# The HUD and the overlays draw from a ~14 KB glyph subset of Source Han Sans
# CN (OFL-1.1). Regenerate it after changing any on-screen string.
HUD_CHARS := 祖玛分数关卡第通过游戏结束胜利重新开始连击最高分下发射瞄准继续并命中剩余暂停点击扳机左键球速新纪录危险快吃掉了洞口或按得零一二三四五六七八九十0123456789xR .,:+-/!?

font:
	pyftsubset /usr/share/fonts/adobe-source-han-sans/SourceHanSansCN-Bold.otf \
		--text="$(HUD_CHARS)" --output-file=internal/font/SourceHanSansCN-Signs.otf \
		--layout-features='' --no-hinting --desubroutinize --drop-tables+=GSUB,GPOS,GDEF

clean:
	rm -rf bin
