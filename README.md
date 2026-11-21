# Zuma VR (祖玛 VR)

A Zuma-style marble shooter built for **standalone VR on the Steam Frame** — it
runs on the headset itself (ARM64 / SteamOS, one self-contained executable, no
PC tether and no asset files) and also opens a desktop window on a PC for
development.

You are the launcher: beads leave your **right controller** along a 3D ballistic
arc, a cyan beam shows where they will land, and runs of three or more pop.
Clear the board to advance; let the head of the chain reach the hole and the
round is over.

![Title screen](docs/title.png)

![Aiming from the right controller](docs/vr-hero.png)

Everything is procedural — meshes, the carved groove, the board shapes, the
title-screen poster, the HUD font subset and all sound effects are generated at
start-up.

## Built for the Steam Frame

- **Standalone**: `make frame` cross-compiles `bin/zuma-frame` for ARM64
  SteamOS; copy that one file to the headset and it runs there, rendering
  1728×1728 per eye with 4× MSAA at ~120 fps.
- **Hand-tracked aiming**: the launcher sits in the right hand; aim by pointing
  the controller, fire with the trigger. The ball is a real projectile with
  gravity, not a raycast hit.
- **Room-scale presentation**: the playfield stands on a table in front of you,
  tilted back 20° like a pinball playfield; the title screen is anchored at a
  fixed spot in the room, so looking around moves the view past it instead of
  dragging it along.
- **The table frame**: the table has its own coordinate system — its origin is
  the centre of the felt, "table height" is the distance from that origin down
  to the floor (**0.50 m** by default), and the tilt turns the whole table about
  the same origin (**20°** by default). Both are on the controller: the right
  stick moves the height between 0.15 m and 1.65 m, the left stick trims the
  tilt between flat and 35°. The score header and the win/lose text keep their
  own upright frame: they stand at 90° to the floor whatever the table angle.
- The desktop window is the development/mirror view of the left eye.

### Screenshots

| In play | Win | Lose |
| --- | --- | --- |
| ![Aim](docs/vr-hero.png) | ![Win](docs/vr-win.png) | ![Lose](docs/vr-lose.png) |

*(Documentation shots are rendered by the same path with `-vrcam`, which poses
the right hand and its beam.)*

## Controls

| Action | Steam Frame (VR) | Desktop (development) |
| --- | --- | --- |
| Start from the title screen | Either trigger (that hand keeps the launcher) | `Space` / left click |
| Take the launcher | Trigger on the other hand | — |
| Aim | Point the controller holding the launcher | Move the mouse over the board |
| Fire | Trigger on the launcher hand | Left click / `Space` |
| Restart after a loss | Trigger on the launcher hand | `R` |
| Load the next level after a win | Trigger on the launcher hand | `Space` / left click |
| Raise / lower the table (height above the floor) | Right stick up / down | — |
| Tilt the table (about its centre) | Left stick up / down (up = flatter) | — |
| Quit | SteamVR menu | `Esc` |

The loaded bead rides at the controller; there is no reticle at the end of the
beam on purpose — the beam itself stops on the bead it will hit. Only the hand
that holds the launcher draws the beam and the bead: pull the other hand's
trigger to pass the launcher over, pull the launcher's own trigger to shoot.

## Rules

- The chain runs along one generated route: a single-line spiral, up to two
  nested rings, a maze or arc switchback, a wavy hairpin, a square spiral, or a
  one-stroke silhouette (heart-free outlines such as star, apple, pentagon,
  eye, cup). Board shapes, route family, direction and rotation are re-rolled
  every level; a guard rail rejects layouts whose non-adjacent sections come
  closer than one bead, that leave the felt, or that are too short.
- A shot inserts on the side it struck; three or more touching beads of one
  colour pop. Points are `10 × beads × combo`.
- **Combo is per shot**: every extra pop caused by the same shot raises it. The
  chain recoils away from the hole from x2 onwards, harder with every level,
  and the impulse only lands when the two halves actually meet again.
- Gaps close from the **front** section (the side nearer the hole), so a clear
  can pull the next beads into a new match — chain reactions come from there.
- The launcher only hands you colours that are **still on the chain**; a colour
  that has been cleared completely is not dealt until it spawns again. The band
  painted around the felt wears the colour of the bead in hand, so the launcher
  and the table always agree.
- Losing: the chain's head reaches the hole. Winning: board and queue empty —
  the round then waits for you. The trigger fires a volley of thirty ordinary
  beads — every colour, random angles inside a 20° cone around your aim ray,
  same speed and arc as a normal shot — with a bang, and the next level loads a
  moment later.

## Difficulty curve

Every round is tuned by `game.SpecFor(level)`: three dials that grow linearly
and then cap.

| Level | Beads to clear | Colours | Chain speed (m/s) | Beads pre-laid |
| --- | --- | --- | --- | --- |
| 1 | 45 | 4 | 0.200 | 16 |
| 2 | 53 | 4 | 0.222 | 18 |
| 3 | 61 | 5 | 0.244 | 20 |
| 4 | 69 | 5 | 0.266 | 22 |
| 5 | 77 | 6 | 0.288 | 24 |
| 6 | 85 | 6 | 0.310 | 26 |
| 7 | 90 (cap) | 6 | 0.332 | 28 |
| 8–15 | 90 | 6 | +0.022 per level | +2 per level |
| 16+ | 90 | 6 | 0.52 (cap) | 45 (cap) |

- **Beads** `45 + 8 × (level − 1)`, capped at 90.
- **Colours** `4 + (level − 1) / 2`, capped at 6 — a new colour every two
  levels, which is what actually makes matches harder to line up.
- **Chain speed** `0.20 + 0.022 × (level − 1)` m/s, capped at 0.52, i.e. 2.6×
  the opening pace.
- **Beads pre-laid** `16 + 2 × (level − 1)`, never more than half the round's
  beads: the board already carries a chain heading for the hole on the first
  frame.

What deliberately does **not** scale:

- **Aiming and shooting** — 14.8 m/s with at most two beads in the air, at
  every level.
- **Geometry** — board shape and route family are rolled fresh every round from
  the same weights and the same guard rails (route ≥ 7.2 m, non-adjacent
  sections at least one bead apart, inside the felt). A late level is not a
  longer or twistier course, just a busier one.
- **Combo recoil** — one shared impulse curve, capped at 9 m/s whatever the
  level.
- **Scoring** — `10 × beads × combo` per pop, plus `150 × level` for clearing.

Beads sit 0.124 m apart, so a route holds roughly 58–113 of them. At the
90-bead cap that makes a short route noticeably harder than a long one: late
rounds vary with the layout roll as well as with the level.

## Build and run

Dependencies (Arch names): `glfw`, `glew`, `openal`, `openvr`, `go`. For the
headset build you also need the aarch64 cross toolchain and a user sysroot
(`SYS_ARM64`, default `~/.local/aarch64-sysroot`; same setup as the author's
`govr`).

### Steam Frame (the target platform)

```sh
make frame                                   # -> bin/zuma-frame (ARM64)
scp bin/zuma-frame frame:Games/zuma/         # `frame` = ssh alias of the headset
ssh frame 'cd ~/Games/zuma && DISPLAY=:0 XDG_RUNTIME_DIR=/run/user/1000 \
           setsid nohup ./zuma-frame -vr-debug >run.log 2>&1 </dev/null &'
```

The log prints `VR active: 1728x1728 per eye, msaa 4x`, a per-level
`board …: tilt …, felt near/centre/far …` line, and a once-a-second pose line.
Only the binary is shipped: `make frame` links against the headset's own
`libGLEW.so.2.2` (copied to `~/.local/glew22`, override with `GLEW22_DIR`) so
its `NEEDED` entry matches the system library and no `.so` sits next to it.

### PC (development / mirror)

```sh
make desktop     # windowed view, mouse aims on the board
make run         # same binary, VR when a headset is attached
make test        # unit tests (rules, routes, meshes, text, aiming)
make shot        # refresh the documentation screenshots
```

## Flags

| Flag | Meaning |
| --- | --- |
| `-desktop` | Force the flat window view even with a headset connected |
| `-vr=false` | Do not touch SteamVR at all |
| `-title=false` | Skip the title screen and drop straight into level 1 |
| `-screenshot out.png -frames N` | Render on a fixed clock, write one frame, exit |
| `-demo won\|lost` | Put the round in a finished state for screenshots |
| `-seed N` | Reproduce a particular layout |
| `-vrcam` | Documentation framing: pose the right hand and its beam |
| `-controller-model=false` | Skip the controller mesh. OpenXR never draws the hands for the app, so the game does — and the beam leaves from the mesh's front vertex |

## Layout

```
cmd/zuma         window, input, VR presentation, screenshots
internal/game    pure rules: routes, board shapes, chain, shots, matches, scoring
internal/render  shaders, procedural meshes, board/table/bead drawing, HUD text
internal/splashart  the painted title-screen art (embedded as PNG)
internal/font    Source Han Sans CN subset and a rasteriser
internal/audio   OpenAL glue plus synthesised clips
internal/gl      thin cgo bindings to the GL 4.1 calls used here
internal/vr      cgo wrapper around the SteamVR/OpenVR runtime
```

`internal/gl` and `internal/vr` are the small cgo glue packages shared with the
author's `govr` project; they are vendored so the game builds on its own.

The HUD font is a glyph subset of Source Han Sans CN (SIL OFL 1.1, see
`internal/font/LICENSE-SourceHanSans.txt`). Regenerate it with `make font` after
changing any on-screen text.

---

## 中文说明

一个为 **Steam Frame 一体机**做的祖玛式弹珠游戏：编译出的单个 ARM64 可执行文件
直接跑在头显里（SteamOS，无需连电脑、没有资源文件），同时也能在 PC 上开一个
桌面窗口用于开发调试。

你就是炮台：彩球从**右手柄**沿 3D 弹道射出，青色射线标出落点，同色三颗以上消除。
清空盘面过关，球链头部滚进洞口则本局结束。

- **一体机**：`make frame` 交叉编译 `bin/zuma-frame`，拷到头显就能跑；每眼
  1728×1728、4× MSAA、约 120 fps。
- **手柄操作**：待发的球握在右手，指向即瞄准，扣扳机发射（真弹道，带重力）；
  射线末端不画准星——射线停在哪颗球上就插在哪。
- **场景**：盘面立在面前的桌上，像弹球台一样后仰 20°；标题页固定在房间里某个
  坐标，转头不会拖着它走。桌面窗口只是左眼的调试镜像。
- 桌面有自己的坐标系：原点取毡面中心，**桌面高度＝原点到地面的距离（默认 0.50 m）**，倾角绕这个原点旋转（默认 20°）。右手摇杆上下调高度（0.15–1.65 m），左手摇杆上下调倾角（向上更平、向下更陡，0–35°）；桌子与盘面作为同一个刚体一起转。HUD 信息与结算文字独占一个「直立坐标系」——在世界里始终与地面垂直，不跟桌面倾角走。
- 盘面外圈那条装饰线跟着**手上的球色**走（换球即换色），看一眼盘沿就知道下一颗是什么颜色。
- 过关后结算页会等你扣扳机：一击沿射线方向、20° 圆锥内随机角度喷出 30 颗各色球，
  就是普通的空射（同样速度、同样弹道），伴随一声「砰」，随后进入下一关。
- 规则：线路/盘形每关随机（单线螺旋、最多两层环、迷宫/圆弧发卡、波浪发卡、
  方形螺旋、一笔画轮廓），洞口位置随机；插入按命中侧、三消、**连击按每发计**、
  缺口由靠洞口一侧回退合拢、击退随连击级别加重；手上只发链上还有的颜色。
- 难度曲线：只靠三个线性增长的参数（`game.SpecFor`）——要清的球数
  `45 + 8×(关-1)`（上限 90）、颜色数 `4 + (关-1)/2`（上限 6，每两关多一色）、
  链速 `0.20 + 0.022×(关-1)` m/s（上限 0.52），外加开局已铺的球数
  `16 + 2×(关-1)`（不超过本关球数的一半）。射击手感（14.8 m/s、最多两颗在飞）、
  盘形与线路（每关同权重重抽、守门条件不变）、连击击退都不随关卡变化，所以
  第 7 关球数、第 5 关颜色、第 16 关链速到顶之后曲线是平的，后段难度更多来自
  随机盘面的运气（线路越短、越早被填满）。
- 全部资源程序化生成（网格、凹槽、盘形、标题页海报、字体子集、音效），
  只有一个可执行文件。

构建：`make desktop`（窗口）/ `make run`（有头显即进 VR）/ `make frame`（头显
ARM64）/ `make test`。
