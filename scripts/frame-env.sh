#!/bin/sh
# Runs a command with the cgo environment pointed at a user-space ARM64 sysroot.
#
# Cross compiling for Steam Frame needs ARM64 copies of the GL/X11/openal/
# openvr headers and libs. Drop them under <sysroot>/usr (see README)
# and this wires the include, library and pkg-config paths, then adds an
# $ORIGIN rpath so a libopenvr_api.so shipped next to the binary is found.
#
# Usage: scripts/frame-env.sh [sysroot] command ...

set -eu

sysroot=${1:-}
[ $# -gt 0 ] && shift

if [ -n "$sysroot" ] && [ -d "$sysroot/usr/lib" ]; then
	CGO_CFLAGS="-O2 -g0 -ffunction-sections -fdata-sections -I$sysroot/usr/include"
	CGO_CXXFLAGS="-O2 -g0 -ffunction-sections -fdata-sections -I$sysroot/usr/include -I$sysroot/usr/include/openvr"
	# --allow-shlib-undefined: the OpenVR SDK's libopenvr_api.so leaves
	# vr::CleanupInternalInterfaces for the SteamVR runtime to provide.
	CGO_LDFLAGS="-L$sysroot/usr/lib -Wl,-rpath-link,$sysroot/usr/lib -Wl,-rpath,\$ORIGIN -Wl,--allow-shlib-undefined -Wl,--gc-sections -Wl,--as-needed"
	PKG_CONFIG_SYSROOT_DIR="$sysroot"
	PKG_CONFIG_LIBDIR="$sysroot/usr/lib/pkgconfig:$sysroot/usr/share/pkgconfig"
	export CGO_CFLAGS CGO_CXXFLAGS CGO_LDFLAGS PKG_CONFIG_SYSROOT_DIR PKG_CONFIG_LIBDIR
fi

exec "$@"
