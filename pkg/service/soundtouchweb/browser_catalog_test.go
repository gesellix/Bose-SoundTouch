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

const catalogFixtureScript = `
import { h, render } from 'preact';
import { Presets } from '/app/static/js/components/Presets.js';
const status = {
  revision: 1,
  nowPlaying: { Source: 'STANDBY' },
  presets: { Preset: [
    { ID: 1, ContentItem: { Source: 'TUNEIN', SourceAccount: '', Location: 's12345', ItemName: 'WDR 2' } },
  ] },
};
render(h('section', { id: 'presets' }, h(Presets, { deviceId: 'speaker', status })), document.getElementById('fixture'));
`

// TestPresetSlotFillsFromTheCatalog covers the issue 754 pick list: a slot is
// filled from what AfterTouch has seen, rather than from hand-edited XML. It
// asserts the request the picker sends, that an entry already in a slot says
// so instead of being hidden, and that the filter narrows the list.
func TestPresetSlotFillsFromTheCatalog(t *testing.T) {
	var mu sync.Mutex
	var stored []string

	server := newPlayerFixtureServer(t, catalogFixtureScript, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true,
				"entries": []any{
					map[string]any{
						"source": "RADIO_BROWSER", "location": "uuid-fm4", "name": "FM4",
						"origin": "recent", "last_seen": "2026-09-20T12:05:00Z",
					},
					// The speaker echoes the source name back as sourceAccount
					// in recents; the preset above leaves it empty. The picker
					// has to see through that to say "in preset 1".
					map[string]any{
						"source": "TUNEIN", "source_account": "TUNEIN", "location": "s12345", "name": "WDR 2",
						"origin": "preset", "last_seen": "2026-09-20T12:00:00Z",
					},
				},
			}})
		})
		r.Post("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			stored = append(stored, chi.URLParam(req, "slot")+" "+string(body))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var occupiedMeta string
	var filtered int

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.Click(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`, chromedp.ByQuery),

		// Wait for the list itself, not just the panel: the entries arrive
		// with the catalog fetch, and clicking before they render would hit a
		// node that is about to be replaced.
		chromedp.WaitVisible(`#presets .catalog-entry`, chromedp.ByQuery),

		// The entry that already occupies slot 1 is listed, and says so.
		chromedp.Evaluate(`Array.from(document.querySelectorAll('#presets .catalog-entry-meta'))
			.map(e => e.textContent).find(t => t.includes('WDR 2') || t.includes('TuneIn')) || ''`, &occupiedMeta),

		// The filter narrows the list rather than reloading it.
		chromedp.SendKeys(`#presets .catalog-picker-filter`, "FM4", chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('#presets .catalog-entry').length === 1`, nil),
		chromedp.Evaluate(`document.querySelectorAll('#presets .catalog-entry').length`, &filtered),

		chromedp.Click(`#presets .catalog-entry`, chromedp.ByQuery),

		// A filled slot closes the picker.
		chromedp.Poll(`document.querySelector('#presets .catalog-picker') === null`, nil),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if !strings.Contains(occupiedMeta, "in preset 1") {
		t.Errorf("expected the entry already in a slot to say so, got %q", occupiedMeta)
	}

	if filtered != 1 {
		t.Errorf("expected the filter to leave 1 entry, got %d", filtered)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(stored) != 1 {
		t.Fatalf("expected exactly one preset write, got %v", stored)
	}

	for _, want := range []string{`2 {`, `"source":"RADIO_BROWSER"`, `"location":"uuid-fm4"`, `"itemName":"FM4"`} {
		if !strings.Contains(stored[0], want) {
			t.Errorf("preset write %q should contain %q", stored[0], want)
		}
	}
}

// A standalone soundtouch-player has no catalog behind it. Offering an empty
// pick list there would read as "nothing seen yet", which is a different thing.
func TestCatalogPickerSaysWhenThereIsNoCatalog(t *testing.T) {
	server := newPlayerFixtureServer(t, catalogFixtureScript, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": false,
				"entries":   []any{},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var note string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.Click(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`, chromedp.ByQuery),
		// Poll for the settled note rather than the first one: the panel shows
		// "Loading…" in the same element while the catalog request is in
		// flight, so reading it straight away races the fetch.
		chromedp.Poll(`!document.querySelector('#presets .catalog-picker-note')?.textContent.includes('Loading')`, nil),
		chromedp.Text(`#presets .catalog-picker-note`, &note, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if !strings.Contains(note, "no catalog") {
		t.Errorf("expected the picker to say there is no catalog, got %q", note)
	}
}

// TestPresetSlotClearsWithConfirmation covers the one editor action that
// destroys something. It is offered only for a slot that holds something, it
// asks first, and what it says while asking is the reason it is safe: the
// station stays on the pick list.
func TestPresetSlotClearsWithConfirmation(t *testing.T) {
	var mu sync.Mutex
	var cleared []string

	server := newPlayerFixtureServer(t, catalogFixtureScript, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true,
				"entries": []any{map[string]any{
					"source": "TUNEIN", "location": "s12345", "name": "WDR 2",
					"origin": "preset", "last_seen": "2026-09-20T12:00:00Z",
				}},
			}})
		})
		r.Delete("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			cleared = append(cleared, chi.URLParam(req, "slot"))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var emptySlotOffersClear bool
	var confirmText string

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),

		// Slot 2 is empty in the fixture: there is nothing to empty, so the
		// action is not offered at all.
		chromedp.WaitVisible(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.Click(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`#presets .catalog-entry`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#presets .catalog-clear-btn') !== null`, &emptySlotOffersClear),

		// Slot 1 holds WDR 2.
		chromedp.Click(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`#presets .catalog-clear-btn`, chromedp.ByQuery),
		chromedp.Click(`#presets .catalog-clear-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`#presets .catalog-picker-confirm`, chromedp.ByQuery),
		chromedp.Text(`#presets .catalog-picker-foot .catalog-picker-note`, &confirmText, chromedp.ByQuery),
		chromedp.Click(`#presets .catalog-picker-confirm .catalog-clear-btn.danger`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#presets .catalog-picker') === null`, nil),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if emptySlotOffersClear {
		t.Error("an empty slot must not offer to be emptied")
	}

	if !strings.Contains(confirmText, "stays on this list") {
		t.Errorf("the confirmation should say the station is kept, got %q", confirmText)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(cleared) != 1 || cleared[0] != "1" {
		t.Fatalf("expected preset 1 to be cleared once, got %v", cleared)
	}
}

// TestPresetSlotRenamesAndMoves covers the two edits that do not need a new
// endpoint: both are a store of a named ContentItem, with a different name or
// into a different slot. The move also asserts the order the two requests go
// in, which is what decides whether a half-failed move loses the station.
func TestPresetSlotRenamesAndMoves(t *testing.T) {
	var mu sync.Mutex
	var calls []string

	server := newPlayerFixtureServer(t, catalogFixtureScript, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true,
				"entries":   []any{},
			}})
		})
		r.Post("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			calls = append(calls, "store "+chi.URLParam(req, "slot")+" "+string(body))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Delete("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			calls = append(calls, "clear "+chi.URLParam(req, "slot"))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
	})

	ctx := newHeadlessChromeContext(t)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),

		// Slot 1 holds WDR 2. Rename it.
		chromedp.WaitVisible(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.Click(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`#presets .catalog-rename-input`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => {
			const input = document.querySelector('#presets .catalog-rename-input');
			const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
			setter.call(input, 'WDR 2 Rheinland');
			input.dispatchEvent(new Event('input', { bubbles: true }));
			return true;
		})()`, nil),
		chromedp.Poll(`!document.querySelector('#presets .catalog-action-btn').disabled`, nil),
		chromedp.Click(`#presets .catalog-action-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#presets .catalog-picker') === null`, nil),

		// Move slot 1 to the empty slot 3: no confirmation, since nothing
		// would be replaced.
		chromedp.Click(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`#presets .catalog-move-slots`, chromedp.ByQuery),
		chromedp.Click(`#presets .catalog-move-slot:nth-child(2)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#presets .catalog-picker') === null`, nil),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 3 {
		t.Fatalf("expected a rename store plus a move (store then clear), got %v", calls)
	}

	if !strings.HasPrefix(calls[0], "store 1 ") || !strings.Contains(calls[0], `"itemName":"WDR 2 Rheinland"`) {
		t.Errorf("rename should store the new name into the same slot, got %q", calls[0])
	}

	if !strings.HasPrefix(calls[1], "store 3 ") {
		t.Errorf("a move must store into the target first, got %q", calls[1])
	}

	if calls[2] != "clear 1" {
		t.Errorf("a move must clear the source second, got %q", calls[2])
	}
}
