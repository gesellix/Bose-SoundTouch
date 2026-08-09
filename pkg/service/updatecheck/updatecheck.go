// Package updatecheck implements the opt-in periodic check against a
// GitHub repo's latest release (#591,
// _/i591/design-update-check.md). Deliberately generic (repo and current
// version are constructor arguments, not hardcoded) and decoupled from
// handlers.Server/main.go globals, so other binaries could construct their
// own Checker later without a rewrite — see the design doc's answer to
// open question 2 (CLI-only users).
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

const defaultTimeout = 5 * time.Second

const defaultBaseURL = "https://api.github.com"

// Result is the outcome of the most recent check.
type Result struct {
	Available      bool      `json:"available"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	ReleaseURL     string    `json:"release_url,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
}

// Checker checks a GitHub repo's latest release against the running
// version. Safe for concurrent use.
type Checker struct {
	mu             sync.RWMutex
	repo           string // "owner/repo"
	currentVersion string
	httpClient     *http.Client
	ds             *datastore.DataStore
	baseURL        string
	last           Result
}

// NewChecker constructs a Checker for repo (e.g. "gesellix/Bose-SoundTouch")
// against currentVersion, seeding its last-known result from ds's persisted
// UpdateCheckState if present (so a restart doesn't lose "already knew
// about vX.Y.Z" until the next tick). ds may be nil (state just won't
// persist across restarts).
func NewChecker(ds *datastore.DataStore, repo, currentVersion string) *Checker {
	c := &Checker{
		repo:           repo,
		currentVersion: currentVersion,
		httpClient:     &http.Client{Timeout: defaultTimeout},
		ds:             ds,
		baseURL:        defaultBaseURL,
		last:           Result{CurrentVersion: currentVersion},
	}

	if ds == nil {
		return c
	}

	state, err := ds.GetUpdateCheckState()
	if err != nil || state.LastSeenVersion == "" {
		return c
	}

	c.last.LatestVersion = state.LastSeenVersion
	c.last.ReleaseURL = state.LastReleaseURL

	if ts, parseErr := time.Parse(time.RFC3339, state.LastCheckedAt); parseErr == nil {
		c.last.CheckedAt = ts
	}

	if normalizedCurrent, ok := normalizeVersion(currentVersion); ok {
		if normalizedLatest, ok2 := normalizeVersion(state.LastSeenVersion); ok2 {
			c.last.Available = semver.Compare(normalizedLatest, normalizedCurrent) > 0
		}
	}

	return c
}

// SetBaseURL overrides the GitHub API base URL. Test-only — not exposed via
// config, since there's exactly one GitHub to check against in production.
func (c *Checker) SetBaseURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.baseURL = url
}

// SetTimeout overrides the HTTP client timeout (production always uses
// defaultTimeout). Test-only, to exercise timeout handling without a
// multi-second test.
func (c *Checker) SetTimeout(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.httpClient.Timeout = d
}

// LastResult returns the outcome of the most recent check (or the
// persisted-state-seeded value if CheckNow hasn't run yet this process).
func (c *Checker) LastResult() Result {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.last
}

// CheckNow performs one check against the GitHub API, updates LastResult,
// and persists the outcome (if ds is non-nil and a latest version was
// found). Returns an error only on a genuine fetch/parse failure — a
// current version that can't be meaningfully compared (dev/(devel)/dirty
// builds) is not an error, it's a no-op "skip the check" result, per the
// design doc's answer on non-release builds.
func (c *Checker) CheckNow(ctx context.Context) (Result, error) {
	result := Result{CurrentVersion: c.currentVersion, CheckedAt: time.Now()}

	normalizedCurrent, ok := normalizeVersion(c.currentVersion)
	if !ok {
		c.setLast(result)
		return result, nil
	}

	release, err := c.fetchLatestRelease(ctx)
	if err != nil {
		return Result{}, err
	}

	if !release.Prerelease {
		result.LatestVersion = release.TagName
		result.ReleaseURL = release.HTMLURL

		if normalizedLatest, ok := normalizeVersion(release.TagName); ok {
			result.Available = semver.Compare(normalizedLatest, normalizedCurrent) > 0
		}
	}

	c.setLast(result)
	c.persist(result)

	return result, nil
}

func (c *Checker) setLast(result Result) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.last = result
}

// persist saves the outcome, but only when a latest version was actually
// found — a transient fetch failure (already excluded, CheckNow returns
// before calling this) or a defensive prerelease-only response must not
// overwrite previously-known-good state with emptiness.
func (c *Checker) persist(result Result) {
	if c.ds == nil || result.LatestVersion == "" {
		return
	}

	_ = c.ds.SaveUpdateCheckState(datastore.UpdateCheckState{
		LastCheckedAt:   result.CheckedAt.UTC().Format(time.RFC3339),
		LastSeenVersion: result.LatestVersion,
		LastReleaseURL:  result.ReleaseURL,
	})
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	HTMLURL    string `json:"html_url"`
}

func (c *Checker) fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	c.mu.RLock()
	baseURL := c.baseURL
	c.mu.RUnlock()

	url := fmt.Sprintf("%s/repos/%s/releases/latest", baseURL, c.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}

	req.Header.Set("User-Agent", "AfterTouch-update-check")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("github releases API returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return githubRelease{}, fmt.Errorf("read response: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return githubRelease{}, fmt.Errorf("parse response: %w", err)
	}

	return release, nil
}

// normalizeVersion reports whether v can be meaningfully compared as
// semver, and its normalized ("v"-prefixed) form if so. Deliberately
// treats dev/(devel)/dirty builds as unparseable rather than guessing —
// see the design doc.
func normalizeVersion(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" || v == "(devel)" || strings.Contains(v, "dirty") {
		return "", false
	}

	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}

	if !semver.IsValid(v) {
		return "", false
	}

	return v, true
}
