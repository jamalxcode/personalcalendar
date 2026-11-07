package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// appBundleName is the app's file name on disk.
const appBundleName = "Smart Cal.app"

// renameBundle renames an older "Personal Calendar.app" to "Smart Cal.app" in place (once).
// A running app can be renamed safely on macOS; the new name shows in Finder right away.
func renameBundle() bool {
	b, err := currentBundle()
	if err != nil || filepath.Base(b) == appBundleName {
		return false
	}
	target := filepath.Join(filepath.Dir(b), appBundleName)
	if _, err := os.Stat(target); err == nil {
		return false // something already has that name; leave both alone
	}
	return os.Rename(b, target) == nil
}

// version is set at build time from the VERSION file (-X main.version=...).
var version = "0.0.0"

const releaseRepo = "jamalxcode/personalcalendar"

// UpdateInfo describes the newest GitHub release compared to this build.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Notes     string `json:"notes"`
	Published string `json:"published"`
	PageURL   string `json:"pageUrl"`
	assetURL  string
	Checked   time.Time `json:"checked"`
}

func checkLatest(ctx context.Context) (UpdateInfo, error) {
	info := UpdateInfo{Current: version, Checked: time.Now()}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+releaseRepo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PersonalCalendar-updater")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return info, fmt.Errorf("couldn't reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return info, fmt.Errorf("no releases published yet")
	}
	if resp.StatusCode != 200 {
		return info, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var rel struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return info, err
	}
	info.Latest = strings.TrimPrefix(rel.TagName, "v")
	info.Notes = rel.Body
	info.PageURL = rel.HTMLURL
	info.Published = rel.PublishedAt
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, "-macos-arm64.zip") {
			info.assetURL = a.URL
		}
	}
	info.Available = info.assetURL != "" && newerVersion(info.Latest, info.Current)
	return info, nil
}

// newerVersion reports whether a > b for dotted numeric versions ("1.2.10" > "1.2.9").
func newerVersion(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(strings.TrimFunc(pa[i], func(r rune) bool { return r < '0' || r > '9' }))
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(strings.TrimFunc(pb[i], func(r rune) bool { return r < '0' || r > '9' }))
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// currentBundle returns the path of the running app bundle.
func currentBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	} else if _, statErr := os.Stat(exe); statErr != nil {
		// our own program file is gone: an update already replaced this copy on disk
		return "", fmt.Errorf("Smart Cal was already updated on disk — click Quit app, then open Smart Cal from Applications")
	}
	b := filepath.Dir(filepath.Dir(filepath.Dir(exe))) // …/X.app/Contents/MacOS/bin → …/X.app
	if !strings.HasSuffix(b, ".app") {
		return "", fmt.Errorf("not running from an .app bundle (%s)", exe)
	}
	return b, nil
}

// installUpdate downloads the release zip, unpacks it, and hands off to a tiny script that
// swaps the bundle once this process has quit, then relaunches the new version.
// Files downloaded by the app itself (not a browser) carry no quarantine flag, so Gatekeeper
// doesn't block the ad-hoc-signed build.
func installUpdate(ctx context.Context, info UpdateInfo, progress func(string)) error {
	if info.assetURL == "" {
		return fmt.Errorf("no download for this release")
	}
	bundle, err := currentBundle()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "personalcalendar-update-")
	if err != nil {
		return err
	}
	progress("Downloading v" + info.Latest + "…")
	zipPath := filepath.Join(tmp, "update.zip")
	if err := download(ctx, info.assetURL, zipPath); err != nil {
		return err
	}
	progress("Unpacking…")
	unz := filepath.Join(tmp, "unzipped")
	if out, err := exec.Command("ditto", "-x", "-k", zipPath, unz).CombinedOutput(); err != nil {
		return fmt.Errorf("unzip failed: %s", strings.TrimSpace(string(out)))
	}
	// the zip holds one .app (named "Personal Calendar.app" so older versions' updaters still work)
	newApp := ""
	if ents, err := os.ReadDir(unz); err == nil {
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".app") {
				newApp = filepath.Join(unz, e.Name())
			}
		}
	}
	if _, err := os.Stat(filepath.Join(newApp, "Contents", "MacOS", "PersonalCalendar")); newApp == "" || err != nil {
		return fmt.Errorf("the download doesn't contain the app")
	}
	target := filepath.Join(filepath.Dir(bundle), appBundleName) // installs as "Smart Cal.app"
	script := filepath.Join(tmp, "swap.sh")
	body := fmt.Sprintf(`#!/bin/bash
PID=%d
OLD=%q
NEW=%q
TARGET=%q
for i in $(seq 1 150); do kill -0 $PID 2>/dev/null || break; sleep 0.1; done
kill -0 $PID 2>/dev/null && { kill -9 $PID 2>/dev/null; sleep 0.5; }   # old copy still up: stop it so "open" starts the new one
rm -rf "$OLD.previous"
mv "$OLD" "$OLD.previous" && mv "$NEW" "$TARGET" || { mv "$OLD.previous" "$OLD" 2>/dev/null; open "$OLD"; exit 1; }
rm -rf "$OLD.previous"
xattr -dr com.apple.quarantine "$TARGET" 2>/dev/null
open "$TARGET"
`, os.Getpid(), bundle, newApp, target)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("/bin/bash", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // survive our exit
	if err := cmd.Start(); err != nil {
		return err
	}
	progress("Restarting…")
	return nil
}

func download(ctx context.Context, url, dest string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "PersonalCalendar-updater")
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
