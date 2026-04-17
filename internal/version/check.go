package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// checkInterval is how often we check for a new version.
	checkInterval = 24 * time.Hour

	// releaseURL is the GitHub API endpoint for the latest release.
	releaseURL = "https://api.github.com/repos/Bandwidth/cli/releases/latest"
)

// stateFile tracks the last check time and latest known version.
type stateFile struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version"`
}

// CheckResult is returned when a newer version is available.
type CheckResult struct {
	Current string
	Latest  string
}

// Check compares the running version against the latest GitHub release.
// Returns nil if the version is current, the check was performed recently,
// or anything goes wrong (version checks should never block the user).
func Check(currentVersion string) *CheckResult {
	// Never check for dev builds
	if currentVersion == "dev" {
		return nil
	}

	// Respect BW_NO_UPDATE_NOTIFIER for CI/scripts
	if os.Getenv("BW_NO_UPDATE_NOTIFIER") != "" {
		return nil
	}

	stateDir, err := stateDir()
	if err != nil {
		return nil
	}
	statePath := filepath.Join(stateDir, "update-check.json")

	// Read cached state
	state := loadState(statePath)

	// If we checked recently, use the cached result
	if time.Since(state.LastCheck) < checkInterval {
		if state.LatestVersion != "" && isNewer(currentVersion, state.LatestVersion) {
			return &CheckResult{Current: currentVersion, Latest: state.LatestVersion}
		}
		return nil
	}

	// Fetch latest version from GitHub (with a short timeout)
	latest, err := fetchLatestVersion()
	if err != nil {
		return nil
	}

	// Cache the result
	state.LastCheck = time.Now()
	state.LatestVersion = latest
	saveState(statePath, state)

	if isNewer(currentVersion, latest) {
		return &CheckResult{Current: currentVersion, Latest: latest}
	}
	return nil
}

// NoticeMessage returns a user-friendly upgrade notice.
func (r *CheckResult) NoticeMessage() string {
	return fmt.Sprintf("A new version of band is available: %s → %s\nUpdate with: brew upgrade band  or  go install github.com/Bandwidth/cli/cmd/band@latest", r.Current, r.Latest)
}

// fetchLatestVersion hits the GitHub releases API and returns the tag name.
func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(releaseURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return strings.TrimPrefix(release.TagName, "v"), nil
}

// isNewer returns true if latest is a newer version than current.
// Simple string comparison after normalizing — works for semver.
func isNewer(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	if current == latest {
		return false
	}
	return compareSemver(current, latest) < 0
}

// compareSemver compares two semver strings. Returns -1, 0, or 1.
// Handles pre-release tags: 0.0.3-beta < 0.0.3.
func compareSemver(a, b string) int {
	aParts, aPre := splitPrerelease(a)
	bParts, bPre := splitPrerelease(b)

	// Compare major.minor.patch
	for i := 0; i < 3; i++ {
		av, bv := 0, 0
		if i < len(aParts) {
			fmt.Sscanf(aParts[i], "%d", &av)
		}
		if i < len(bParts) {
			fmt.Sscanf(bParts[i], "%d", &bv)
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}

	// Same version numbers — pre-release is lower than release
	if aPre != "" && bPre == "" {
		return -1
	}
	if aPre == "" && bPre != "" {
		return 1
	}
	// Both have pre-release: lexicographic
	if aPre < bPre {
		return -1
	}
	if aPre > bPre {
		return 1
	}
	return 0
}

// splitPrerelease splits "1.2.3-beta" into (["1","2","3"], "beta").
func splitPrerelease(v string) ([]string, string) {
	pre := ""
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		pre = v[idx+1:]
		v = v[:idx]
	}
	return strings.Split(v, "."), pre
}

// stateDir returns the directory for storing version check state.
func stateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "band")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func loadState(path string) stateFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return stateFile{}
	}
	var s stateFile
	json.Unmarshal(data, &s)
	return s
}

func saveState(path string, s stateFile) {
	data, _ := json.Marshal(s)
	os.WriteFile(path, data, 0o644)
}
