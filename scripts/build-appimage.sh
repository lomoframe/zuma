#!/bin/sh
# Builds a self-contained AppImage for zuma.
#
# Usage: scripts/build-appimage.sh <amd64|arm64> [binary] [output]
#
# The AppImage carries the libraries a SteamOS host does not ship with the
# program (OpenAL and the headset's libGLEW.so.2.2); GL, X11, libstdc++ and glibc
# stay on the host so its own graphics driver and C++ runtime are used. The
# OpenVR/OpenXR clients are NOT bundled either: they must match the running
# SteamVR, and the binary already carries RUNPATH
# $ORIGIN:/opt/steamvr/bin/linuxarm64.
#
# Needs mksquashfs (squashfs-tools). The small AppImage runtime is cached under
# ~/.cache/zuma-appimage and downloaded from AppImageKit on first use.

set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ $# -lt 1 ]; then
	echo "usage: $0 <amd64|arm64> [binary] [output]" >&2
	exit 2
fi

arch=$1
case "$arch" in
amd64)
	runtime_arch=x86_64
	default_bin=bin/zuma
	libdir=/usr/lib
	extra_libdir=
	libs="libopenal.so.1 libGLEW.so.2.3 libstdc++.so.6 libgcc_s.so.1"
	;;
arm64)
	runtime_arch=aarch64
	default_bin=bin/zuma-frame
	libdir=${SYS_ARM64:-$HOME/.local/aarch64-sysroot}/usr/lib
	extra_libdir=$(dirname "$(readlink -f "$(aarch64-linux-gnu-g++ -print-file-name=libstdc++.so.6)")")
	# The headset ships libGLEW.so.2.2 and the game links that SONAME, so the
	# 2.2 build is what has to travel with the bundle (not the sysroot's 2.3).
	# libstdc++/libgcc_s are left to the host: SteamOS carries them and the
	# toolchain's copies are built against a newer glibc than the device has.
	# FRAME_LIBDIR holds libraries copied off the headset itself, so their glibc
	# requirements always match it (the sysroot's libopenal needs GLIBC_2.43).
	libs="libGLEW.so.2.2 libopenal.so.1"
	glibc_ceiling=2.39
	frame_libdir=${FRAME_LIBDIR:-$HOME/.local/frame-libs}
	;;
*)
	echo "unsupported architecture: $arch" >&2
	exit 2
	;;
esac
glew22_dir=${GLEW22_DIR:-$HOME/.local/glew22}

bin=${2:-$default_bin}
out=${3:-dist/zuma-linux-$arch.AppImage}
icon=${APPIMAGE_ICON:-packaging/zuma-icon.png}

command -v mksquashfs >/dev/null 2>&1 || {
	echo "mksquashfs not found; install squashfs-tools" >&2
	exit 1
}
[ -x "$bin" ] || {
	echo "missing executable: $bin" >&2
	exit 1
}
[ -f "$icon" ] || {
	echo "missing icon: $icon" >&2
	exit 1
}

runtime_dir=${APPIMAGE_RUNTIME_DIR:-$HOME/.cache/zuma-appimage}
runtime=$runtime_dir/runtime-$runtime_arch
if [ ! -f "$runtime" ]; then
	mkdir -p "$runtime_dir"
	echo "downloading AppImage runtime-$runtime_arch" >&2
	curl -L --fail --retry 3 -o "$runtime" \
		"https://github.com/AppImage/AppImageKit/releases/download/continuous/runtime-$runtime_arch"
	chmod 0755 "$runtime"
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/zuma-appimage.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
appdir=$tmp/AppDir
squashfs=$tmp/zuma.squashfs

mkdir -p "$appdir/usr/bin" "$appdir/usr/lib" \
	"$appdir/usr/share/applications" \
	"$appdir/usr/share/icons/hicolor/512x512/apps"

install -m 0755 "$bin" "$appdir/usr/bin/zuma"
for lib in $libs; do
	src=
	for dir in "$frame_libdir" "$libdir" "$extra_libdir" "$glew22_dir"; do
		if [ -n "$dir" ] && [ -e "$dir/$lib" ]; then
			src=$dir/$lib
			break
		fi
	done
	if [ -z "$src" ]; then
		echo "missing library: $lib (looked in $frame_libdir $libdir $extra_libdir $glew22_dir)" >&2
		exit 1
	fi
	cp -L "$src" "$appdir/usr/lib/$lib"
	if [ -n "$glibc_ceiling" ]; then
		newer=$(aarch64-linux-gnu-objdump -T "$appdir/usr/lib/$lib" 2>/dev/null |
			grep -o 'GLIBC_2\.[0-9]*' | sort -Vu | tail -1)
		if [ -n "$newer" ] && [ "$newer" != "GLIBC_$glibc_ceiling" ] &&
			[ "$(printf '%s\n%s\n' "GLIBC_$glibc_ceiling" "$newer" | sort -V | tail -1)" != "GLIBC_$glibc_ceiling" ]; then
			echo "$lib needs $newer but the headset has glibc $glibc_ceiling; copy it off the device into $frame_libdir" >&2
			exit 1
		fi
	fi
done

install -m 0644 "$icon" "$appdir/zuma.png"
install -m 0644 "$icon" "$appdir/usr/share/icons/hicolor/512x512/apps/zuma.png"
cp "$icon" "$appdir/.DirIcon"

cat > "$appdir/zuma.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=zuma VR
GenericName=Marble Shooter
Comment=Zuma-style marble shooter built for standalone VR on the Steam Frame
Exec=zuma
Icon=zuma
Terminal=false
Categories=Game;
Keywords=game;vr;zuma;marble;shooter;
StartupWMClass=zuma
EOF
install -m 0644 "$appdir/zuma.desktop" "$appdir/usr/share/applications/zuma.desktop"
ln -sf zuma.desktop "$appdir/.desktop"

cat > "$appdir/AppRun" <<'EOF'
#!/bin/sh
# Bundled libraries first; GL, X11 and glibc still come from the host.
HERE=$(dirname "$(readlink -f "$0")")
export LD_LIBRARY_PATH="$HERE/usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
exec "$HERE/usr/bin/zuma" "$@"
EOF
chmod 0755 "$appdir/AppRun"

mkdir -p "$(dirname "$out")"
rm -f "$out"
# Pin every timestamp so the same inputs produce the same bytes.
: "${SOURCE_DATE_EPOCH:=0}"
export SOURCE_DATE_EPOCH
mksquashfs "$appdir" "$squashfs" -root-owned -noappend -quiet -no-progress
cat "$runtime" "$squashfs" > "$out"
chmod 0755 "$out"
echo "$out"
