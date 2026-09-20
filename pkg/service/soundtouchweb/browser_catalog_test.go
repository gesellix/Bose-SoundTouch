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
		chromedp.WaitVisible(`#presets .catalog-picker-note`, chromedp.ByQuery),
		chromedp.Text(`#presets .catalog-picker-note`, &note, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if !strings.Contains(note, "no catalog") {
		t.Errorf("expected the picker to say there is no catalog, got %q", note)
	}
}
