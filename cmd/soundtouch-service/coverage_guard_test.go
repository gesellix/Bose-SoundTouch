package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/go-chi/chi/v5"
)

// frozenFirstSegments are the top-level path prefixes that belong to the frozen
// speaker / app contract (category 1a/1b in
// docs/content/docs/architecture/API-ROUTE-LAYOUT.md). Routes under these must
// not change shape across the issue #451 refactor, so each should have at least
// one .http contract test (the suite under tests/integration/http-client/, run
// by `make test-http-client`). Movable surfaces (/setup, /mgmt, /web) and infra
// (/, /health, /docs, /favicon.ico) are intentionally excluded.
var frozenFirstSegments = map[string]bool{
	"streaming": true,
	"accounts":  true,
	"customer":  true,
	"bmx":       true,
	"bmx-icons": true,
	"core02":    true,
	"oauth":     true,
	"custom":    true,
	"media":     true,
	"updates":   true,
	"v1":        true,
	"alexa":     true,
	"ced":       true,
}

func coverageFirstSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}

	return p
}

// patternToRegexp converts a chi route pattern into an anchored regexp:
// `{param}` becomes a single path segment (`[^/]+`) and `*` becomes `.*`.
func patternToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder

	b.WriteString("^")

	for i, seg := range strings.Split(pattern, "/") {
		if i > 0 {
			b.WriteString("/")
		}

		switch {
		case seg == "*":
			b.WriteString(".*")
		case strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}"):
			b.WriteString("[^/]+")
		default:
			b.WriteString(regexp.QuoteMeta(seg))
		}
	}

	b.WriteString("$")

	return regexp.MustCompile(b.String())
}

// loadHTTPClientRequests extracts (method, path) pairs from every .http file in
// the integration suite. `{{host}}` is stripped (leaving a leading `/`), query
// strings are dropped, and `{{var}}` template segments are left intact (they
// contain no slash, so they match a `[^/]+` route segment).
func loadHTTPClientRequests(t *testing.T, dir string) [][2]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read http-client dir %s: %v", dir, err)
	}

	reqLine := regexp.MustCompile(`^\s*(GET|POST|PUT|DELETE|PATCH|HEAD)\s+(\S+)`)

	var out [][2]string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".http") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}

		for _, line := range strings.Split(string(data), "\n") {
			m := reqLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			url := strings.ReplaceAll(m[2], "{{host}}", "")
			if i := strings.IndexByte(url, '?'); i >= 0 {
				url = url[:i]
			}

			if !strings.HasPrefix(url, "/") {
				continue
			}

			out = append(out, [2]string{m[1], url})
		}
	}

	return out
}

// TestFrozenRouteContractCoverage enforces that every frozen-contract route the
// service registers is exercised by at least one .http integration test. The
// set of *uncovered* frozen routes is golden-filed: adding a new frozen route
// without a test (or adding a test that newly covers one) changes the set and
// fails this test, forcing a conscious update of the golden file. It is the
// machine-checked companion to tests/integration/http-client/COVERAGE.md.
func TestFrozenRouteContractCoverage(t *testing.T) {
	server := handlers.NewServer(nil, nil, "http://localhost:8000", true, true, true)
	r := setupRouter(server, nil)

	httpRequests := loadHTTPClientRequests(t, filepath.Join("..", "..", "tests", "integration", "http-client"))

	// Only the request methods the contract suite actually exercises. Routes
	// registered via chi HandleFunc carry every method (CONNECT/TRACE/...); those
	// extra verbs are noise for coverage purposes.
	meaningfulMethods := map[string]bool{
		http.MethodGet: true, http.MethodPost: true, http.MethodPut: true, http.MethodDelete: true,
	}

	var uncovered []string

	walkFunc := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !meaningfulMethods[method] {
			return nil
		}

		if !frozenFirstSegments[coverageFirstSegment(route)] {
			return nil
		}

		re := patternToRegexp(route)
		for _, req := range httpRequests {
			if req[0] == method && re.MatchString(req[1]) {
				return nil
			}
		}

		uncovered = append(uncovered, fmt.Sprintf("%-7s %s", method, route))

		return nil
	}

	if err := chi.Walk(r, walkFunc); err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	sort.Strings(uncovered)
	output := strings.Join(uncovered, "\n") + "\n"

	const goldenPath = "testdata/frozen_routes_uncovered.txt"

	actualPath := "testdata/frozen_routes_uncovered.actual.txt"
	if err := os.WriteFile(actualPath, []byte(output), 0644); err != nil {
		t.Fatalf("write actual: %v", err)
	}

	golden, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		if err := os.WriteFile(goldenPath, []byte(output), 0644); err != nil {
			t.Fatalf("create golden: %v", err)
		}

		t.Logf("created golden %s with %d uncovered frozen routes", goldenPath, len(uncovered))

		return
	}

	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if string(golden) != output {
		t.Errorf("Frozen-route contract coverage changed.\n"+
			"A frozen route either lost its .http test or a new one was added without one.\n"+
			"Review and, if intended, update %s from %s.", goldenPath, actualPath)
	}
}
