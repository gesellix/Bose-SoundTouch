//go:build browsertest

package soundtouchweb

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

// The fixture is the issue 697 shape: the speaker reports six presets, and the
// service stores eight rows for it.
const storedPresetsFixtureScript = `
import { h, render } from 'preact';
import { Presets } from '/app/static/js/components/Presets.js';
const status = {
  revision: 1,
  nowPlaying: { Source: 'STANDBY' },
  presets: { Preset: [1, 2, 3, 4, 5, 6].map(id => ({ ID: id, ContentItem: { Source: 'TUNEIN', Location: 's' + id, ItemName: 'Station ' + id } })) },
};
render(h('section', { id: 'presets' }, h(Presets, { deviceId: 'speaker', status })), document.getElementById('fixture'));
`

func storedRow(index int, button, name, source, verdict string) map[string]any {
	return map[string]any{
		"index": index, "button": button, "name": name,
		"source": source, "verdict": verdict,
	}
}

func TestStoredPresetProblemsAreShownAndRepaired(t *testing.T) {
	var mu sync.Mutex
	var repairs []string

	rows := []any{}
	for i := 1; i <= 6; i++ {
		rows = append(rows, storedRow(i-1, string(rune('0'+i)), "Station "+string(rune('0'+i)), "TUNEIN", "ok"))
	}

	rows = append(rows,
		storedRow(6, "7", "Leftover", "TUNEIN", "out-of-range"),
		storedRow(7, "", "Orphan", "TUNEIN", "no-slot"),
	)

	server := newPlayerFixtureServer(t, storedPresetsFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/stored-presets/", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			repaired := len(repairs) > 0
			mu.Unlock()

			data := map[string]any{
				"available": true, "rows": rows, "unrecallable": 2,
				"speaker_count": 6, "disagrees": true,
			}
			if repaired {
				data = map[string]any{
					"available": true, "rows": rows[:6], "unrecallable": 0,
					"speaker_count": 6, "disagrees": false,
				}
			}

			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: data})
		})
		r.Post("/api/control/devices/speaker/stored-presets/repair", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			repairs = append(repairs, string(body))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true, "rows": rows[:6], "unrecallable": 0,
				"speaker_count": 6, "disagrees": false,
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var headline string
	var rowsShownBeforeOpening int
	var removeTitle string
	var bodyText string

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#presets .stored-presets-summary`, chromedp.ByQuery),
		chromedp.Text(`#presets .stored-presets-headline`, &headline, chromedp.ByQuery),

		// The problem announces itself; the detail waits to be asked for.
		chromedp.Evaluate(`document.querySelectorAll('#presets .stored-presets-row').length`, &rowsShownBeforeOpening),

		chromedp.Click(`#presets .stored-presets-summary`, chromedp.ByQuery),
		chromedp.WaitVisible(`#presets .stored-presets-fix`, chromedp.ByQuery),

		// Where a removal lands has to be legible without hovering: a touch
		// device has no hover, so the tooltip cannot be the only carrier.
		chromedp.Evaluate(`document.querySelector('#presets .stored-presets-remove').title`, &removeTitle),
		chromedp.Text(`#presets .stored-presets-body`, &bodyText, chromedp.ByQuery),

		chromedp.Click(`#presets .stored-presets-fix`, chromedp.ByQuery),

		// A repaired list stops warning at all.
		chromedp.Poll(`document.querySelector('#presets .stored-presets') === null`, nil),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if !strings.Contains(headline, "8") || !strings.Contains(headline, "2") {
		t.Errorf("the headline should name both counts, got %q", headline)
	}

	if rowsShownBeforeOpening != 0 {
		t.Errorf("expected the rows to stay collapsed until asked for, got %d", rowsShownBeforeOpening)
	}

	if !strings.Contains(removeTitle, "AfterTouch stores") || !strings.Contains(removeTitle, "speaker") {
		t.Errorf("the remove tooltip should name what it removes from, got %q", removeTitle)
	}

	if !strings.Contains(bodyText, "deletes the row from what AfterTouch stores") {
		t.Errorf("the panel should say where a removal lands without hovering, got %q", bodyText)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(repairs) != 1 {
		t.Fatalf("expected one repair request, got %v", repairs)
	}

	// The two unplayable rows by position, and the count the view was built
	// from, so a repair against a stale view is refused rather than applied.
	for _, want := range []string{`"drop":[6,7]`, `"expected":8`} {
		if !strings.Contains(repairs[0], want) {
			t.Errorf("repair body %q should contain %q", repairs[0], want)
		}
	}
}

// A single row can be removed on its own, which is what the surplus-of-valid-
// rows case needs: nothing is "broken", the list is simply longer than the
// speaker, and only the owner knows which row to drop.
func TestASingleStoredRowCanBeRemoved(t *testing.T) {
	var mu sync.Mutex
	var repairs []string

	server := newPlayerFixtureServer(t, storedPresetsFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/stored-presets/", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true,
				"rows": []any{
					storedRow(0, "1", "Station 1", "TUNEIN", "ok"),
					storedRow(1, "2", "Station 2", "TUNEIN", "ok"),
				},
				"unrecallable": 0, "speaker_count": 1, "disagrees": true,
			}})
		})
		r.Post("/api/control/devices/speaker/stored-presets/repair", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			repairs = append(repairs, string(body))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true, "rows": []any{storedRow(0, "1", "Station 1", "TUNEIN", "ok")},
				"unrecallable": 0, "speaker_count": 1, "disagrees": false,
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var offersBulkFix bool

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#presets .stored-presets-summary`, chromedp.ByQuery),
		chromedp.Click(`#presets .stored-presets-summary`, chromedp.ByQuery),
		chromedp.WaitVisible(`#presets .stored-presets-row`, chromedp.ByQuery),

		// Nothing here is unplayable, so there is no bulk fix to offer.
		chromedp.Evaluate(`document.querySelector('#presets .stored-presets-fix') !== null`, &offersBulkFix),

		chromedp.Click(`#presets .stored-presets-row:nth-child(2) .stored-presets-remove`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#presets .stored-presets') === null`, nil),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if offersBulkFix {
		t.Error("a list with no unplayable row must not offer to remove unplayable rows")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(repairs) != 1 || !strings.Contains(repairs[0], `"drop":[1]`) {
		t.Fatalf("expected the second row to be dropped by position, got %v", repairs)
	}
}

// A healthy install must show nothing at all.
func TestNoWarningWhenTheStoredListAgrees(t *testing.T) {
	server := newPlayerFixtureServer(t, storedPresetsFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/stored-presets/", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true, "rows": []any{}, "unrecallable": 0,
				"speaker_count": 6, "disagrees": false,
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var warned bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#presets .preset-grid`, chromedp.ByQuery),
		// Wait for the request itself to have happened, so "nothing rendered"
		// is a verdict on the answer rather than on the timing.
		chromedp.Poll(`performance.getEntriesByType('resource').some(e => e.name.includes('stored-presets'))`, nil),
		chromedp.Evaluate(`document.querySelector('#presets .stored-presets') !== null`, &warned),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if warned {
		t.Error("a stored list that agrees with the speaker must not warn")
	}
}
