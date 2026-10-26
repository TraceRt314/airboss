#!/usr/bin/env bash
# build-notifier.sh <dest-dir> — builds Airboss.app (the native notifier) into
# <dest-dir>: compiles the Swift helper, draws the icon, bundles the system
# sounds and signs it ad hoc, which UserNotifications requires. Needs the Xcode
# command line tools (swiftc).
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST="${1:?usage: build-notifier.sh <dest-dir>}"
VERSION="$(git -C "$HERE" describe --tags --always 2>/dev/null || echo dev)"
APP="$DEST/Airboss.app"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
swiftc -O -swift-version 5 -o "$APP/Contents/MacOS/airboss-notifier" "$HERE/notifier/main.swift"
sed "s|__VERSION__|$VERSION|" "$HERE/notifier/Info.plist" > "$APP/Contents/Info.plist"

swiftc -O -swift-version 5 -o "$TMP/make-icon" "$HERE/notifier/make-icon.swift"
"$TMP/make-icon" "$TMP/icon.png"
mkdir "$TMP/AppIcon.iconset"
for s in 16 32 128 256 512; do
  sips -z "$s" "$s" "$TMP/icon.png" --out "$TMP/AppIcon.iconset/icon_${s}x${s}.png" >/dev/null
  sips -z "$((s * 2))" "$((s * 2))" "$TMP/icon.png" --out "$TMP/AppIcon.iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns -o "$APP/Contents/Resources/AppIcon.icns" "$TMP/AppIcon.iconset"

# UNNotificationSound(named:) looks in the app bundle, not /System/Library/Sounds
cp /System/Library/Sounds/*.aiff "$APP/Contents/Resources/"

codesign --force --sign - "$APP" >/dev/null 2>&1
# a rebuilt bundle at the same path: tell LaunchServices so the icon refreshes
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$APP" >/dev/null 2>&1 || true
echo "$APP"
