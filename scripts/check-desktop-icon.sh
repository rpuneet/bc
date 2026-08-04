#!/usr/bin/env sh
# check-desktop-icon.sh — refuse to ship a desktop build that fell back to the
# Wails default "W" icon.
#
# Wails' buildassets.ReadFile silently writes its embedded default appicon.png
# (the black-on-white "W") when desktop/build/appicon.png is missing, then
# encodes that into iconfile.icns. A local make build-local-desktop can
# therefore produce a Dock icon that looks nothing like mycel, while the
# committed mushroom PNG is still sitting in git (#3605).
#
# Usage:
#   check-desktop-icon.sh              verify source appicon.png only
#   check-desktop-icon.sh --packaged   also verify the packaged darwin icns
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
APPICON="$ROOT/desktop/build/appicon.png"
# Content hash of github.com/wailsapp/wails/v2@v2.13.0/pkg/buildassets/build/appicon.png
WAILS_DEFAULT_PNG_MD5=ea40194a9e0523b4be1d45f1378106bc
# icns produced by jackmordaunt/icns@v1.0.0 from that default PNG
WAILS_DEFAULT_ICNS_MD5=0a5f5a572786d23927d843c6f1dd5eb9

md5_of() {
	if command -v md5 >/dev/null 2>&1; then
		md5 -q "$1"
	else
		md5sum "$1" | awk '{print $1}'
	fi
}

if [ ! -f "$APPICON" ]; then
	echo "error: missing $APPICON" >&2
	echo "Wails will silently substitute its default \"W\" icon. Restore the mycel mushroom PNG from git." >&2
	exit 1
fi

png_md5=$(md5_of "$APPICON")
if [ "$png_md5" = "$WAILS_DEFAULT_PNG_MD5" ]; then
	echo "error: $APPICON is the Wails default \"W\" icon ($png_md5)" >&2
	echo "Replace it with the mycel mushroom mark (desktop/build/appicon.svg → appicon.png)." >&2
	exit 1
fi

if [ "${1:-}" = "--packaged" ]; then
	icns=""
	for candidate in \
		"$ROOT/desktop/build/bin/mycel.app/Contents/Resources/iconfile.icns" \
		"$ROOT/desktop/build/bin/mycel-desktop.app/Contents/Resources/iconfile.icns"
	do
		if [ -f "$candidate" ]; then
			icns=$candidate
			break
		fi
	done
	if [ -z "$icns" ]; then
		echo "error: packaged iconfile.icns not found under desktop/build/bin/*.app" >&2
		exit 1
	fi
	icns_md5=$(md5_of "$icns")
	if [ "$icns_md5" = "$WAILS_DEFAULT_ICNS_MD5" ]; then
		echo "error: packaged $icns is the Wails default \"W\" icns ($icns_md5)" >&2
		echo "desktop/build/appicon.png was missing or wrong at package time (#3605)." >&2
		exit 1
	fi
	echo "desktop icon ok (png=$png_md5 icns=$icns_md5)"
else
	echo "desktop appicon.png ok ($png_md5)"
fi
