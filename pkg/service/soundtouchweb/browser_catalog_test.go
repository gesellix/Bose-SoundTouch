//go:build browsertest

package soundtouchweb

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

const catalogFixtureScript = `
import { h, render } from 'preact';
import { Presets } from '/app/static/js/components/Presets.js';
const status = {
  epoch: 1,
  revision: 1,
  nowPlaying: { Source: 'STANDBY' },
  presets: { Preset: [
    { ID: 1, ContentItem: { Source: 'TUNEIN', SourceAccount: '', Location: 's12345', ItemName: 'WDR 2' } },
  ] },
};
window.renderPresetFixture = (items = status.presets.Preset) => render(h('section', { id: 'presets' }, h(Presets, {
  deviceId: 'speaker', physicalId: 'SPEAKER', status: { ...status, presets: { Preset: items } },
})), document.getElementById('fixture'));
renderPresetFixture();
`

const catalogSlotOne = `{"ID":1,"ContentItem":{"Source":"TUNEIN","Location":"s12345","ItemName":"WDR 2"}}`

func writePresetFixtureBank(w http.ResponseWriter, entries ...string) {
	_, _ = io.WriteString(w, `{"success":true,"data":{"info":{"device_id":"SPEAKER"},"status":{"epoch":1,"presets":{"Preset":[`+
		strings.Join(entries, ",")+`]}}}}`)
}

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
		r.Get("/api/control/devices/speaker/presets", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if len(stored) == 0 {
				writePresetFixtureBank(w, catalogSlotOne)
				return
			}
			writePresetFixtureBank(w, catalogSlotOne, `{"ID":2,"ContentItem":{"Source":"RADIO_BROWSER","Location":"uuid-fm4","ItemName":"FM4"}}`)
		})
	})

	ctx := newHeadlessChromeContext(t)

	var occupiedMeta string
	var filtered int

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`)),

		// Wait for the list itself, not just the panel: the entries arrive
		// with the catalog fetch, and clicking before they render would hit a
		// node that is about to be replaced.
		chromedp.WaitVisible(chromedp.CSS(`#presets .catalog-entry`)),

		// The entry that already occupies slot 1 is listed, and says so.
		evalInto(`Array.from(document.querySelectorAll('#presets .catalog-entry-meta'))
			.map(e => e.textContent).find(t => t.includes('WDR 2') || t.includes('TuneIn')) || ''`, &occupiedMeta),

		// The filter narrows the list rather than reloading it.
		chromedp.SendKeys(chromedp.CSS(`#presets .catalog-picker-filter`), "FM4"),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('#presets .catalog-entry').length === 1`),
		evalInto(`document.querySelectorAll('#presets .catalog-entry').length`, &filtered),

		chromedp.Click(chromedp.CSS(`#presets .catalog-entry`)),

		// A filled slot closes the picker.
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .catalog-picker') === null`),
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
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`)),
		// Poll for the settled note rather than the first one: the panel shows
		// "Loading…" in the same element while the catalog request is in
		// flight, so reading it straight away races the fetch.
		chromedp.Poll[chromedp.Void](`!document.querySelector('#presets .catalog-picker-note')?.textContent.includes('Loading')`),
		textInto(chromedp.CSS(`#presets .catalog-picker-note`), &note),
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
		r.Get("/api/control/devices/speaker/presets", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if len(cleared) == 0 {
				writePresetFixtureBank(w, catalogSlotOne)
				return
			}
			writePresetFixtureBank(w)
		})
	})

	ctx := newHeadlessChromeContext(t)

	var emptySlotOffersClear bool
	var confirmText string

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),

		// Slot 2 is empty in the fixture: there is nothing to empty, so the
		// action is not offered at all.
		chromedp.WaitVisible(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(2) .preset-edit-btn`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .catalog-entry`)),
		evalInto(`document.querySelector('#presets .catalog-clear-btn') !== null`, &emptySlotOffersClear),

		// Slot 1 holds WDR 2.
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .catalog-clear-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .catalog-clear-btn`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .catalog-picker-confirm`)),
		textInto(chromedp.CSS(`#presets .catalog-picker-foot .catalog-picker-note`), &confirmText),
		chromedp.Click(chromedp.CSS(`#presets .catalog-picker-confirm .catalog-clear-btn.danger`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .catalog-picker') === null`),
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

func TestPresetClearKeepsEditorOpenWhenSpeakerDoesNotClear(t *testing.T) {
	var deletes atomic.Int32
	server := newPlayerFixtureServer(t, catalogFixtureScript, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"success":true,"data":{"available":true,"entries":[]}}`)
		})
		r.Get("/api/control/devices/speaker/presets", func(w http.ResponseWriter, _ *http.Request) {
			writePresetFixtureBank(w, catalogSlotOne)
		})
		r.Delete("/api/control/devices/speaker/preset/1", func(w http.ResponseWriter, _ *http.Request) {
			deletes.Add(1)
			_, _ = io.WriteString(w, `{"success":true}`)
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .catalog-clear-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .catalog-picker-confirm .catalog-clear-btn.danger`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('.catalog-picker [role="alert"]')?.textContent.includes('Preset change unverified')`),
	); err != nil {
		t.Fatalf("the unchanged slot should leave the editor open with a warning: %v", err)
	}
	if got := deletes.Load(); got != 1 {
		t.Fatalf("clear sent %d writes, want 1", got)
	}
}

// TestPresetSlotRenamesAndMoves verifies the destination before clearing the
// source and verifies the resulting bank before closing the editor.
func TestPresetSlotRenamesAndMoves(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var renamed, moved, cleared bool

	server := newPlayerFixtureServer(t, catalogFixtureScript, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true,
				"entries":   []any{},
			}})
		})
		r.Post("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			slot := chi.URLParam(req, "slot")
			mu.Lock()
			calls = append(calls, "store "+slot+" "+string(body))
			if slot == "1" {
				renamed = true
			}
			if slot == "3" {
				moved = true
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Delete("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			calls = append(calls, "clear "+chi.URLParam(req, "slot"))
			cleared = true
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/presets", func(w http.ResponseWriter, req *http.Request) {
			if req.Header.Get("X-AfterTouch-Preset-Target") != "SPEAKER" || req.Header.Get("X-AfterTouch-Preset-Epoch") != "1" {
				w.WriteHeader(http.StatusConflict)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, "read")
			station := `"ContentItem":{"Source":"TUNEIN","Location":"s12345","ItemName":"WDR 2"}`
			if renamed {
				station = `"ContentItem":{"Source":"TUNEIN","Location":"s12345","ItemName":"WDR 2 Rheinland"}`
			}
			var presets []string
			if !cleared {
				presets = append(presets, `{"ID":1,`+station+`}`)
			}
			if moved {
				presets = append(presets, `{"ID":3,`+station+`}`)
			}
			writePresetFixtureBank(w, presets...)
		})
	})

	ctx := newHeadlessChromeContext(t)

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),

		// Slot 1 holds WDR 2. Rename it.
		chromedp.WaitVisible(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .catalog-rename-input`)),
		chromedp.Evaluate[chromedp.Void](`(() => {
			const input = document.querySelector('#presets .catalog-rename-input');
			const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
			setter.call(input, 'WDR 2 Rheinland');
			input.dispatchEvent(new Event('input', { bubbles: true }));
			return true;
		})()`),
		chromedp.Poll[chromedp.Void](`!document.querySelector('#presets .catalog-action-btn').disabled`),
		chromedp.Click(chromedp.CSS(`#presets .catalog-action-btn`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .catalog-picker') === null`),
		chromedp.Evaluate[chromedp.Void](`renderPresetFixture([{ID: 1, ContentItem: {Source: 'TUNEIN', Location: 's12345', ItemName: 'WDR 2 Rheinland'}}])`),

		// Move slot 1 to the empty slot 3: no confirmation, since nothing
		// would be replaced.
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .catalog-move-slots`)),
		chromedp.Click(chromedp.CSS(`#presets .catalog-move-slot:nth-child(2)`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .catalog-picker') === null`),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 8 {
		t.Fatalf("expected a rename store and a verified move, got %v", calls)
	}

	if calls[0] != "read" || !strings.HasPrefix(calls[1], "store 1 ") ||
		!strings.Contains(calls[1], `"itemName":"WDR 2 Rheinland"`) || calls[2] != "read" {
		t.Errorf("rename should verify the new name in the same slot, got %v", calls[:3])
	}

	if calls[3] != "read" || !strings.HasPrefix(calls[4], "store 3 ") || calls[5] != "read" || calls[6] != "clear 1" || calls[7] != "read" {
		t.Errorf("a move must verify before both writes and after clearing the source, got %v", calls)
	}
}

func TestPresetMoveDoesNotConfirmAChangedDestination(t *testing.T) {
	fixture := catalogFixtureScript + `
window.destination = name => renderPresetFixture([...status.presets.Preset, {
  ID: 3, ContentItem: { Source: 'TUNEIN', Location: name, ItemName: name },
}]);
destination('Original station');
`
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"success":true,"data":{"available":true,"entries":[]}}`)
		})
		r.Get("/api/control/devices/speaker/presets", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"success":true,"data":{"info":{"device_id":"SPEAKER"},"status":{"epoch":1,"presets":{"Preset":[
				{"ID":1,"ContentItem":{"Source":"TUNEIN","Location":"s12345","ItemName":"WDR 2"}},
				{"ID":3,"ContentItem":{"Source":"TUNEIN","Location":"Replacement station","ItemName":"Replacement station"}}
			]}}}}`)
		})
		r.MethodFunc(http.MethodPost, "/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, _ *http.Request) {
			t.Error("move overwrote the new destination without confirmation")
			w.WriteHeader(http.StatusConflict)
		})
	})
	ctx := newHeadlessChromeContext(t)
	edit := chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)
	move := chromedp.CSS(`#presets .catalog-move-slot:nth-child(2)`)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"), chromedp.Click(edit), chromedp.Click(move),
		chromedp.Poll[chromedp.Void](`document.querySelector('.catalog-current [role=alert]')?.textContent.includes('Original station')`),
		chromedp.Evaluate[chromedp.Void](`destination('Replacement station')`),
		chromedp.Click(move),
		chromedp.Poll[chromedp.Void](`document.querySelector('.catalog-picker-note.error')?.textContent.includes('source or destination preset changed')`),
	); err != nil {
		t.Fatalf("stale move confirmation: %v", err)
	}
}

// TestCatalogMarksContentThisSpeakerCannotPlay covers the "say so before
// writing" half of the copying check: the catalog is service-wide, so it
// offers entries from speakers with media servers or linked services this one
// does not have. Storing one leaves a slot that fails at play time.
//
// The marker is advisory and the entry stays clickable, because the source
// list the player holds can be minutes old; the service re-checks against the
// speaker and has the final word.
func TestCatalogMarksContentThisSpeakerCannotPlay(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { Presets } from '/app/static/js/components/Presets.js';
const status = {
  epoch: 1,
  revision: 1,
  nowPlaying: { Source: 'STANDBY' },
  presets: { Preset: [] },
  sources: { SourceItem: [
    { Source: 'TUNEIN', SourceAccount: '', Status: 'READY' },
    { Source: 'STORED_MUSIC', SourceAccount: 'uuid:mine/0', Status: 'READY' },
  ] },
};
render(h('section', { id: 'presets' }, h(Presets, { deviceId: 'speaker', physicalId: 'SPEAKER', status })), document.getElementById('fixture'));
`

	var mu sync.Mutex
	var stored []string

	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/catalog", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true,
				"entries": []any{
					// A media server only the other speaker has.
					map[string]any{
						"source": "STORED_MUSIC", "source_account": "uuid:theirs/0",
						"location": "1$7$0", "name": "Album next door", "origin": "preset",
						"last_seen": "2026-09-20T12:05:00Z",
					},
					map[string]any{
						"source": "TUNEIN", "location": "s12345", "name": "WDR 2",
						"origin": "preset", "last_seen": "2026-09-20T12:00:00Z",
					},
				},
			}})
		})
		r.Post("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			stored = append(stored, string(body))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/presets", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if len(stored) == 0 {
				writePresetFixtureBank(w)
				return
			}
			writePresetFixtureBank(w, `{"ID":1,"ContentItem":{"Source":"STORED_MUSIC","SourceAccount":"uuid:theirs/0","Location":"1$7$0","ItemName":"Album next door"}}`)
		})
	})

	ctx := newHeadlessChromeContext(t)

	var marked int
	var markedName, warning string

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)),
		chromedp.Click(chromedp.CSS(`#presets .preset-slot-wrap:nth-child(1) .preset-edit-btn`)),
		chromedp.WaitVisible(chromedp.CSS(`#presets .catalog-entry`)),

		// Exactly the one entry whose account this speaker lacks.
		evalInto(`document.querySelectorAll('#presets .catalog-entry.unavailable').length`, &marked),
		textInto(chromedp.CSS(`#presets .catalog-entry.unavailable .catalog-entry-name`), &markedName),
		textInto(chromedp.CSS(`#presets .catalog-entry-warn`), &warning),

		// Still clickable: the service decides against a fresh source list.
		chromedp.Click(chromedp.CSS(`#presets .catalog-entry.unavailable`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#presets .catalog-picker') === null`),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if marked != 1 {
		t.Errorf("expected exactly one marked entry, got %d", marked)
	}

	if markedName != "Album next door" {
		t.Errorf("marked %q, want the entry from the other speaker's media server", markedName)
	}

	if !strings.Contains(warning, "Library") {
		t.Errorf("the marker should name the source, got %q", warning)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(stored) != 1 {
		t.Fatalf("a marked entry must still be clickable, got %v", stored)
	}
}

// TestSourcesElsewhereListsWhatOtherSpeakersHave covers the read half of the
// sources part of issue 754: what could this speaker be given? The two
// availabilities are the point, because one can be acted on for the owner and
// the other cannot.
func TestSourcesElsewhereListsWhatOtherSpeakersHave(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { Sources } from '/app/static/js/components/Sources.js';
render(h('section', { id: 'sources' }, h(Sources, {
  deviceId: 'speaker',
  status: {
    revision: 1,
    nowPlayingRevision: 1,
    sources: { SourceItem: [{ Source: 'TUNEIN', SourceAccount: '', DisplayName: 'TuneIn', Status: 'READY' }] },
    nowPlaying: { Source: 'STANDBY' },
  },
})), document.getElementById('fixture'));
`

	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/sources-elsewhere", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true,
				"sources": []any{
					map[string]any{
						"type": "STORED_MUSIC", "account": "uuid:theirs/0", "display_name": "fritz",
						"availability": "addable", "devices": []string{"DEVICEID02"},
					},
					map[string]any{
						"type": "SPOTIFY", "account": "listener", "display_name": "Spotify",
						"availability": "link-required", "devices": []string{"DEVICEID02"},
					},
				},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var rows int
	var first, second string

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#sources .sources-elsewhere-row`)),
		evalInto(`document.querySelectorAll('#sources .sources-elsewhere-row').length`, &rows),
		textInto(chromedp.CSS(`#sources .sources-elsewhere-row:nth-child(1)`), &first),
		textInto(chromedp.CSS(`#sources .sources-elsewhere-row:nth-child(2)`), &second),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if rows != 2 {
		t.Fatalf("expected both sources, got %d", rows)
	}

	if !strings.Contains(first, "fritz") || !strings.Contains(first, "can be added here") {
		t.Errorf("first row = %q, want the media server marked as addable", first)
	}

	if !strings.Contains(second, "needs linking on this speaker") {
		t.Errorf("second row = %q, want the music service marked as needing a link", second)
	}
}

// A single-speaker setup, or a player with no service behind it, must show
// nothing rather than an empty heading.
func TestSourcesElsewhereStaysSilentWithNothingToOffer(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { Sources } from '/app/static/js/components/Sources.js';
render(h('section', { id: 'sources' }, h(Sources, {
  deviceId: 'speaker',
  status: {
    revision: 1,
    nowPlayingRevision: 1,
    sources: { SourceItem: [{ Source: 'TUNEIN', SourceAccount: '', DisplayName: 'TuneIn', Status: 'READY' }] },
    nowPlaying: { Source: 'STANDBY' },
  },
})), document.getElementById('fixture'));
`

	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/sources-elsewhere", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true, "sources": []any{},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var shown bool
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#sources .source-btn`)),
		chromedp.Poll[chromedp.Void](`performance.getEntriesByType('resource').some(e => e.name.includes('sources-elsewhere'))`),
		evalInto(`document.querySelector('#sources .sources-elsewhere') !== null`, &shown),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if shown {
		t.Error("nothing to offer must render nothing at all")
	}
}

// TestAddingASourceFromAnotherSpeaker covers the action half: the addable
// kinds get a button, the link-required ones deliberately do not, and a
// successful add re-reads the list so what was added stops being "elsewhere".
func TestAddingASourceFromAnotherSpeaker(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { Sources } from '/app/static/js/components/Sources.js';
render(h('section', { id: 'sources' }, h(Sources, {
  deviceId: 'speaker',
  status: {
    revision: 1,
    nowPlayingRevision: 1,
    sources: { SourceItem: [{ Source: 'TUNEIN', SourceAccount: '', DisplayName: 'TuneIn', Status: 'READY' }] },
    nowPlaying: { Source: 'STANDBY' },
  },
})), document.getElementById('fixture'));
`

	var mu sync.Mutex
	var added []string

	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/sources-elsewhere", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			done := len(added) > 0
			mu.Unlock()

			sources := []any{
				map[string]any{
					"type": "STORED_MUSIC", "account": "uuid:theirs/0", "display_name": "fritz",
					"availability": "addable", "devices": []string{"DEVICEID02"},
				},
				map[string]any{
					"type": "SPOTIFY", "account": "listener", "display_name": "Spotify",
					"availability": "link-required", "devices": []string{"DEVICEID02"},
				},
			}
			if done {
				sources = sources[1:]
			}

			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"available": true, "sources": sources,
			}})
		})
		r.Post("/api/control/devices/speaker/sources-elsewhere/add", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			added = append(added, string(body))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"added": true, "refreshed": true,
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var buttons int

	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#sources .sources-elsewhere-add`)),

		// Only the media server offers to be added; a music service cannot be
		// given to a speaker, so offering a button would be a lie.
		evalInto(`document.querySelectorAll('#sources .sources-elsewhere-add').length`, &buttons),

		chromedp.Click(chromedp.CSS(`#sources .sources-elsewhere-add`)),

		// The list is re-read, so what was added is no longer "elsewhere".
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('#sources .sources-elsewhere-row').length === 1`),
	); err != nil {
		t.Fatalf("browser run: %v", err)
	}

	if buttons != 1 {
		t.Errorf("expected only the addable source to offer a button, got %d", buttons)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(added) != 1 {
		t.Fatalf("expected one add request, got %v", added)
	}

	for _, want := range []string{`"type":"STORED_MUSIC"`, `"account":"uuid:theirs/0"`, `"name":"fritz"`} {
		if !strings.Contains(added[0], want) {
			t.Errorf("add body %q should contain %q", added[0], want)
		}
	}
}
