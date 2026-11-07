#!/bin/bash
# Builds "Smart Cal.app".
#   ./build.sh          build and copy to ~/Desktop
#   ./build.sh --run    …and launch it
#   ./build.sh --ci     build + zip for a GitHub release (used by .github/workflows/release.yml)
set -euo pipefail
cd "$(dirname "$0")"
APP="Smart Cal.app"
MODE="${1:-}"
VERSION=$(tr -d ' \n' < VERSION)
export CGO_ENABLED=1
export MACOSX_DEPLOYMENT_TARGET=14.0

command -v go >/dev/null || { echo "Go not found. Install with: brew install go"; exit 1; }
xcode-select -p >/dev/null 2>&1 || { echo "Xcode command line tools missing. Run: xcode-select --install"; exit 1; }

echo "→ fetching modules"
if [ "$MODE" = "--ci" ]; then go mod download; else go mod tidy; fi
echo "→ compiling v$VERSION"
mkdir -p build
LDFLAGS="-s -w -X main.version=$VERSION"
# local builds also write .diag/status.json next to the source, for debugging without screen access
if [ "$MODE" != "--ci" ]; then LDFLAGS="$LDFLAGS -X main.diagDir=$(pwd)/.diag -X main.buildStamp=$(date +%H:%M:%S)"; fi
# production = embedded runtime; private_mac_apis = transparent webview over the frosted backdrop
go build -tags production,private_mac_apis -ldflags "$LDFLAGS" -o build/PersonalCalendar .

echo "→ bundling"
rm -rf "build/$APP"
mkdir -p "build/$APP/Contents/MacOS" "build/$APP/Contents/Resources"
cp build/PersonalCalendar "build/$APP/Contents/MacOS/PersonalCalendar"

ICONSET=build/icon.iconset
rm -rf "$ICONSET"; mkdir -p "$ICONSET"
./build/PersonalCalendar --write-icon build/icon.png
for s in 16 32 128 256 512; do
  sips -z $s $s build/icon.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  d=$((s*2)); sips -z $d $d build/icon.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "build/$APP/Contents/Resources/icon.icns"

cat > "build/$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Smart Cal</string>
  <key>CFBundleDisplayName</key><string>Smart Cal</string>
  <key>CFBundleExecutable</key><string>PersonalCalendar</string>
  <key>CFBundleIdentifier</key><string>company.sala.personalcalendar</string>
  <key>CFBundleIconFile</key><string>icon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>NSCalendarsFullAccessUsageDescription</key><string>Smart Cal shows your upcoming events in the menu bar and adds the events you type.</string>
  <key>NSCalendarsUsageDescription</key><string>Smart Cal shows your upcoming events in the menu bar and adds the events you type.</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST

codesign --force --deep -s - "build/$APP" >/dev/null 2>&1 || true

if [ "$MODE" = "--ci" ]; then
  ZIP="build/SmartCal-$VERSION-macos-arm64.zip"
  # inside the zip the bundle keeps its old name so v1.6-and-earlier updaters still find it;
  # the app renames itself to "Smart Cal.app" on first launch
  rm -rf build/zip && mkdir -p build/zip
  cp -R "build/$APP" "build/zip/Personal Calendar.app"
  ditto -c -k --keepParent "build/zip/Personal Calendar.app" "$ZIP"
  echo "✓ Packaged: $ZIP"
  exit 0
fi

# quit any running copy (it holds the single-instance lock) and wait for it to exit
osascript -e 'if application id "company.sala.personalcalendar" is running then tell application id "company.sala.personalcalendar" to quit' >/dev/null 2>&1 || true
pkill -f "Contents/MacOS/PersonalCalendar" 2>/dev/null || true
for i in $(seq 1 20); do pgrep -f "Contents/MacOS/PersonalCalendar" >/dev/null || break; sleep 0.25; done
pkill -9 -f "Contents/MacOS/PersonalCalendar" 2>/dev/null || true
rm -rf "$HOME/Desktop/$APP" "$HOME/Desktop/Personal Calendar.app"  # old name before the Smart Cal rename
cp -R "build/$APP" "$HOME/Desktop/"
echo "✓ Built: ~/Desktop/$APP"
if [ "$MODE" = "--run" ]; then
  sleep 0.5
  open "$HOME/Desktop/$APP" || "$HOME/Desktop/$APP/Contents/MacOS/PersonalCalendar" >/dev/null 2>&1 &
fi
exit 0
