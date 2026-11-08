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
  <key>NSRemindersFullAccessUsageDescription</key><string>Smart Cal shows your to-dos next to your events and adds the ones you type.</string>
  <key>NSRemindersUsageDescription</key><string>Smart Cal shows your to-dos next to your events and adds the ones you type.</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST

# ---- Smart Cal Assistant: Siri/Shortcuts actions + Apple Intelligence (Swift, needs Xcode 26) ----
HELPER="Smart Cal Assistant.app"
build_helper() {
  command -v xcodebuild >/dev/null || { echo "  (no Xcode — skipping Siri & Apple Intelligence helper)"; return 1; }
  if ! command -v xcodegen >/dev/null; then
    if [ "$MODE" = "--ci" ] && command -v brew >/dev/null; then brew install xcodegen >/dev/null; else
      echo "  (xcodegen not installed — brew install xcodegen — skipping helper)"; return 1; fi
  fi
  (cd assistant && xcodegen generate --quiet) || return 1
  rm -rf build/assistant
  xcodebuild -project assistant/SmartCalAssistant.xcodeproj -scheme SmartCalAssistant -configuration Release \
    -derivedDataPath build/assistant -quiet CODE_SIGNING_ALLOWED=NO build || return 1
  local out="build/assistant/Build/Products/Release/$HELPER"
  [ -d "$out" ] || return 1
  mkdir -p "build/$APP/Contents/Helpers"
  cp -R "$out" "build/$APP/Contents/Helpers/"
  ls "build/$APP/Contents/Helpers/$HELPER/Contents/Resources/Metadata.appintents" >/dev/null 2>&1 \
    && echo "  ✓ Siri actions metadata included" || echo "  ! no App Intents metadata found"
  codesign --force -s - -r='designated => identifier "company.sala.personalcalendar.assistant"' "build/$APP/Contents/Helpers/$HELPER" >/dev/null 2>&1 \
    || codesign --force -s - "build/$APP/Contents/Helpers/$HELPER"
}
echo "→ building the assistant (Siri + Apple Intelligence)"
if ! build_helper; then
  if [ "$MODE" = "--ci" ]; then echo "✗ assistant build failed"; exit 1; fi
  echo "  continuing without it"
fi

# Ad-hoc signature with a fixed designated requirement (the bundle ID), so macOS keeps the
# Calendar/Reminders permission across updates instead of asking again after every build.
# (Not --deep: the helper above is signed with its own identity and is sealed into this signature.)
codesign --force -s - -r='designated => identifier "company.sala.personalcalendar"' "build/$APP" >/dev/null 2>&1 \
  || codesign --force -s - "build/$APP" >/dev/null 2>&1 || true
codesign -d -r- "build/$APP" 2>&1 | grep -i designated || true

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
