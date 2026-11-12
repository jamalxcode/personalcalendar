package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	sumURL    string    // SHA-256 of the zip (releases from v2.2.0 on)
	Checked   time.Time `json:"checked"`
}

type ghRelease struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// checkLatest finds the newest release: the latest stable one, or — with beta — the highest
// version among recent releases including pre-releases ("2.3.0-beta.1").
func checkLatest(ctx context.Context, beta bool) (UpdateInfo, error) {
	info := UpdateInfo{Current: version, Checked: time.Now()}
	u := "https://api.github.com/repos/" + releaseRepo + "/releases/latest"
	if beta {
		u = "https://api.github.com/repos/" + releaseRepo + "/releases?per_page=20"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
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
	var rel ghRelease
	if beta {
		var list []ghRelease
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return info, err
		}
		best := -1
		for i, r := range list {
			if r.Draft || !hasZip(r) {
				continue
			}
			if best < 0 || compareVersions(r.TagName, list[best].TagName) > 0 {
				best = i
			}
		}
		if best < 0 {
			return info, fmt.Errorf("no releases published yet")
		}
		rel = list[best]
	} else if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
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
		if strings.HasSuffix(a.Name, "-macos-arm64.zip.sha256") {
			info.sumURL = a.URL
		}
	}
	info.Available = info.assetURL != "" && newerVersion(info.Latest, info.Current)
	return info, nil
}

func hasZip(r ghRelease) bool {
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, "-macos-arm64.zip") {
			return true
		}
	}
	return false
}

// newerVersion reports whether a > b ("1.2.10" > "1.2.9", "2.3.0" > "2.3.0-beta.2").
func newerVersion(a, b string) bool { return compareVersions(a, b) > 0 }

// compareVersions compares dotted versions with an optional pre-release part: -1, 0 or 1.
// A pre-release comes before its release: 2.3.0-beta.1 < 2.3.0-beta.2 < 2.3.0.
func compareVersions(a, b string) int {
	split := func(v string) ([]int, string) {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		pre := ""
		if i := strings.IndexByte(v, '-'); i >= 0 {
			v, pre = v[:i], v[i+1:]
		}
		var nums []int
		for _, p := range strings.Split(v, ".") {
			n, _ := strconv.Atoi(strings.TrimFunc(p, func(r rune) bool { return r < '0' || r > '9' }))
			nums = append(nums, n)
		}
		return nums, pre
	}
	na, pa := split(a)
	nb, pb := split(b)
	for i := 0; i < len(na) || i < len(nb); i++ {
		var x, y int
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "": // a release beats its pre-releases
		return 1
	case pb == "":
		return -1
	}
	// both pre-releases: "beta.2" vs "beta.10" piece by piece, numbers as numbers
	xa, xb := strings.Split(pa, "."), strings.Split(pb, ".")
	for i := 0; i < len(xa) || i < len(xb); i++ {
		if i >= len(xa) {
			return -1
		}
		if i >= len(xb) {
			return 1
		}
		ia, ea := strconv.Atoi(xa[i])
		ib, eb := strconv.Atoi(xb[i])
		switch {
		case ea == nil && eb == nil && ia != ib:
			if ia > ib {
				return 1
			}
			return -1
		case (ea == nil) != (eb == nil): // numbers sort before words
			if ea == nil {
				return -1
			}
			return 1
		case ea != nil && xa[i] != xb[i]:
			if xa[i] > xb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
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
// UpdateStep is one progress report while an update installs (sent to the UI as "update:progress").
type UpdateStep struct {
	Stage string `json:"stage"` // checking | downloading | verifying | installing | restarting | error
	Text  string `json:"text"`
	Pct   int    `json:"pct"` // 0–100 for the download, -1 when unknown
}

func installUpdate(ctx context.Context, info UpdateInfo, progress func(UpdateStep)) error {
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
	dl := "Downloading v" + info.Latest + "…"
	progress(UpdateStep{Stage: "downloading", Text: dl, Pct: -1})
	zipPath := filepath.Join(tmp, "update.zip")
	if err := download(ctx, info.assetURL, zipPath, func(pct int) {
		progress(UpdateStep{Stage: "downloading", Text: dl, Pct: pct})
	}); err != nil {
		return err
	}
	progress(UpdateStep{Stage: "verifying", Text: "Checking the download…", Pct: -1})
	if info.sumURL != "" {
		if err := verifySum(ctx, info.sumURL, zipPath); err != nil {
			return err
		}
	}
	progress(UpdateStep{Stage: "installing", Text: "Installing…", Pct: -1})
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
	// make sure the bundle is complete and its signature seal is intact before swapping it in
	if out, err := exec.Command("codesign", "--verify", "--strict", newApp).CombinedOutput(); err != nil {
		return fmt.Errorf("the downloaded app failed its signature check: %s", strings.TrimSpace(string(out)))
	}
	target := filepath.Join(filepath.Dir(bundle), appBundleName) // installs as "Smart Cal.app"
	script := filepath.Join(tmp, "swap.sh")
	body := fmt.Sprintf(`#!/bin/bash
PID=%d
OLD=%s
NEW=%s
TARGET=%s
TMPD=%s
for i in $(seq 1 150); do kill -0 $PID 2>/dev/null || break; sleep 0.1; done
kill -0 $PID 2>/dev/null && { kill -9 $PID 2>/dev/null; sleep 0.5; }   # old copy still up: stop it so "open" starts the new one
rm -rf "$OLD.previous" "$TARGET.previous"
# a second copy already at the target name (e.g. old "Personal Calendar.app" running beside "Smart Cal.app"):
# move it aside, otherwise mv would put the new app *inside* it
if [ "$TARGET" != "$OLD" ] && [ -e "$TARGET" ]; then mv "$TARGET" "$TARGET.previous"; fi
mv "$OLD" "$OLD.previous" && mv "$NEW" "$TARGET" || { mv "$OLD.previous" "$OLD" 2>/dev/null; [ -e "$TARGET.previous" ] && mv "$TARGET.previous" "$TARGET"; open "$OLD"; exit 1; }
rm -rf "$OLD.previous" "$TARGET.previous"
xattr -dr com.apple.quarantine "$TARGET" 2>/dev/null
open "$TARGET"
rm -rf "$TMPD"
`, os.Getpid(), shq(bundle), shq(newApp), shq(target), shq(tmp))
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("/bin/bash", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // survive our exit
	if err := cmd.Start(); err != nil {
		return err
	}
	progress(UpdateStep{Stage: "restarting", Text: "Restarting Smart Cal…", Pct: 100})
	return nil
}

// verifySum compares the zip with the SHA-256 published next to it in the release.
func verifySum(ctx context.Context, url, zipPath string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "PersonalCalendar-updater")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("couldn't fetch the download's checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("couldn't fetch the download's checksum: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return fmt.Errorf("the release's checksum file is empty")
	}
	got, err := fileSHA256(zipPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(fields[0], got) {
		return fmt.Errorf("the download is damaged (checksum mismatch) — try Update again")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// download saves url to dest, reporting whole-percent progress (when the size is known) at most every 150 ms.
func download(ctx context.Context, url, dest string, onPct func(int)) error {
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
	var src io.Reader = resp.Body
	if onPct != nil && resp.ContentLength > 0 {
		src = &progressReader{r: resp.Body, total: resp.ContentLength, onPct: onPct, last: -1}
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	if onPct != nil {
		onPct(100)
	}
	return f.Close()
}

type progressReader struct {
	r     io.Reader
	total int64
	done  int64
	last  int
	at    time.Time
	onPct func(int)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	pct := int(p.done * 100 / p.total)
	if pct > 100 {
		pct = 100
	}
	if pct != p.last && time.Since(p.at) > 150*time.Millisecond {
		p.last, p.at = pct, time.Now()
		p.onPct(pct)
	}
	return n, err
}

// shq single-quotes a path for bash (safe with spaces, $, backticks and non-ASCII names).
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
