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
	"github.com/gesellix/bose-soundtouch/pkg/models"
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
  epoch: 1,
  nowPlaying: { Source: 'STANDBY' },
  presets: { Preset: [1, 2, 3, 4, 5, 6].map(id => ({ ID: id, ContentItem: { Source: 'TUNEIN', Location: 's' + id, ItemName: 'Station ' + id } })) },
};
render(h('section', { id: 'presets' }, h(Presets, { deviceId: 'speaker', physicalId: 'DEVICE1', status })), document.getElementById('fixture'));
`

func writePresetEditFixtureBank(t *testing.T, w http.ResponseWriter, req *http.Request, bank []models.Preset) {
	t.Helper()
	if req.Header.Get("X-AfterTouch-Preset-Target") != "DEVICE1" || req.Header.Get("X-AfterTouch-Preset-Epoch") != "1" {
		t.Errorf("preset read omitted target identity or epoch: %v", req.Header)
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
		"info": map[string]any{"device_id": "DEVICE1"},
		"status": map[string]any{
			"epoch": 1, "presets": models.Presets{Preset: bank},
		},
	}}); err != nil {
		t.Errorf("encode preset bank: %v", err)
	}
}

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

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#presets .stored-presets-summary`)),
		textInto(chromedp.CSS(`#presets .stored-presets-headline`), &headline),

		// The problem announces itself; the detail waits to be asked for.
		evalInto(`document.querySelectorAll('#presets .stored-presets-row').length`, &rowsShownBeforeOpening),

		chromedp.Click(chromedp.CSS(`#presets .stored-presets-summary`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .stored-presets-fix`)),

		// Where a removal lands has to be legible without hovering: a touch
		// device has no hover, so the tooltip cannot be the only carrier.
		evalInto(`document.querySelector('#presets .stored-presets-remove').title`, &removeTitle),
		textInto(chromedp.CSS(`#presets .stored-presets-body`), &bodyText),

		chromedp.Click(chromedp.CSS(`#presets .stored-presets-fix`)),

		// A repaired list stops warning at all.
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .stored-presets') === null`),
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

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#presets .stored-presets-summary`)),
		chromedp.Click(chromedp.CSS(`#presets .stored-presets-summary`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .stored-presets-row`)),

		// Nothing here is unplayable, so there is no bulk fix to offer.
		evalInto(`document.querySelector('#presets .stored-presets-fix') !== null`, &offersBulkFix),

		chromedp.Click(chromedp.CSS(`#presets .stored-presets-row:nth-child(2) .stored-presets-remove`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .stored-presets') === null`),
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
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#presets .preset-grid`)),
		// Wait for the request itself to have happened, so "nothing rendered"
		// is a verdict on the answer rather than on the timing.
		chromedp.Poll[chromedp.Void](`performance.getEntriesByType('resource').some(e => e.name.includes('stored-presets'))`),
		evalInto(`document.querySelector('#presets .stored-presets') !== null`, &warned),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if warned {
		t.Error("a stored list that agrees with the speaker must not warn")
	}
}

// Each choice settles one slot. Keeping ours waits for a fresh preset bank
// before reloading the comparison, and taking the speaker's writes the service.
func TestPerSlotChoiceSettlesOneButton(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var wrote, adopted bool
	var postWriteReads int

	slotPayload := func() map[string]any {
		slots := []any{}
		if postWriteReads < 2 {
			slots = append(slots, map[string]any{
				"slot": 1,
				"ours": map[string]any{
					"present": true, "source": "TUNEIN", "location": "s1", "itemName": "Ours",
				},
				"theirs": map[string]any{
					"present": true, "source": "RADIO_BROWSER", "location": "uuid-fm4", "itemName": "Theirs",
				},
			})
		}
		if !adopted {
			slots = append(slots, map[string]any{
				"slot": 2,
				"ours": map[string]any{
					"present": true, "source": "TUNEIN", "location": "s2", "itemName": "Stored second station",
				},
				"theirs": map[string]any{
					"present": true, "source": "TUNEIN", "location": "s3", "itemName": "Speaker's second station",
				},
			})
		}
		return map[string]any{
			"available": true,
			"rows": []any{
				storedRow(0, "1", "Ours", "TUNEIN", "ok"),
				storedRow(1, "2", "Stored second station", "TUNEIN", "ok"),
			},
			"unrecallable": 0, "speaker_count": 2, "disagrees": len(slots) > 0,
			"slots": slots,
		}
	}

	server := newPlayerFixtureServer(t, storedPresetsFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/stored-presets/", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, "load")
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: slotPayload()})
		})
		r.Get("/api/control/devices/speaker/presets", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, "read")
			if wrote {
				postWriteReads++
			}
			first := &models.ContentItem{Source: "RADIO_BROWSER", Location: "uuid-fm4", ItemName: "Theirs"}
			if postWriteReads >= 2 {
				first = &models.ContentItem{Source: "TUNEIN", Location: "s1", ItemName: "Ours"}
			}
			writePresetEditFixtureBank(t, w, req, []models.Preset{
				{ID: 1, ContentItem: first},
				{ID: 2, ContentItem: &models.ContentItem{Source: "TUNEIN", Location: "s3", ItemName: "Speaker's second station"}},
			})
		})
		r.Post("/api/control/devices/speaker/stored-presets/adopt", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, "adopt "+string(body))
			adopted = true
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: slotPayload()})
		})
		r.Post("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			calls = append(calls, "store "+chi.URLParam(req, "slot")+" "+string(body))
			wrote = true
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var headline, ourName, theirName string

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#presets .stored-presets-summary`)),
		textInto(chromedp.CSS(`#presets .stored-presets-headline`), &headline),
		chromedp.Click(chromedp.CSS(`#presets .stored-presets-summary`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .stored-presets-slot`)),

		textInto(chromedp.CSS(`#presets .stored-presets-side:nth-child(2) .stored-presets-side-name`), &ourName),
		textInto(chromedp.CSS(`#presets .stored-presets-side:nth-child(3) .stored-presets-side-name`), &theirName),

		// Keeping ours pushes the stored content and removes this disagreement
		// when the speaker reports the change. The second slot still differs.
		chromedp.Click(chromedp.CSS(`#presets .stored-presets-side:nth-child(2) .stored-presets-choose`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .stored-presets-slot .stored-presets-button')?.textContent === '2'`),

		// Taking the speaker's writes the stored list, and the panel settles.
		chromedp.Click(chromedp.CSS(`#presets .stored-presets-side:nth-child(3) .stored-presets-choose`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .stored-presets') === null`),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if !strings.Contains(headline, "different things") {
		t.Errorf("headline = %q, want it to name the per-slot difference", headline)
	}

	if ourName != "Ours" || theirName != "Theirs" {
		t.Errorf("sides read %q / %q, want both shown", ourName, theirName)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 7 {
		t.Fatalf("expected to load, read, store, read twice, reload and adopt, got %v", calls)
	}
	if calls[0] != "load" || calls[1] != "read" || calls[3] != "read" || calls[4] != "read" || calls[5] != "load" {
		t.Errorf("preset confirmation did not precede reloading the panel: %v", calls)
	}

	// Keeping ours goes to the speaker's preset store, with our content.
	if !strings.HasPrefix(calls[2], "store 1 ") || !strings.Contains(calls[2], `"itemName":"Ours"`) {
		t.Errorf("keep-ours call = %q", calls[2])
	}

	// Taking the speaker's goes to the service, naming only the slot: the
	// content comes from asking the speaker again, not from this payload.
	if calls[6] != `adopt {"slot":2}` {
		t.Errorf("take-theirs call = %q", calls[6])
	}
}
