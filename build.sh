#!/bin/bash
# Builds "Personal Calendar.app" and copies it to your Desktop.
set -euo pipefail
cd "$(dirname "$0")"
APP="Personal Calendar.app"
export CGO_ENABLED=1

command -v go >/dev/null || { echo "Go not found. Install with: brew install go"; exit 1; }
xcode-select -p >/dev/null 2>&1 || { echo "Xcode command line tools missing. Run: xcode-select --install"; exit 1; }

echo "→ fetching modules"
go mod tidy
echo "→ compiling"
mkdir -p build
# production = embedded runtime; private_mac_apis = transparent webview over the frosted backdrop
go build -tags production,private_mac_apis -ldflags "-s -w" -o build/PersonalCalendar .

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

VERSION=$(git describe --tags --always 2>/dev/null || echo "1.0")
cat > "build/$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Personal Calendar</string>
  <key>CFBundleDisplayName</key><string>Personal Calendar</string>
  <key>CFBundleExecutable</key><string>PersonalCalendar</string>
  <key>CFBundleIdentifier</key><string>company.sala.personalcalendar</string>
  <key>CFBundleIconFile</key><string>icon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST

codesign --force --deep -s - "build/$APP" >/dev/null 2>&1 || true

pkill -x PersonalCalendar 2>/dev/null || true
rm -rf "$HOME/Desktop/$APP"
cp -R "build/$APP" "$HOME/Desktop/"
echo "✓ Built: ~/Desktop/$APP"
[ "${1:-}" = "--run" ] && open "$HOME/Desktop/$APP"
exit 0
