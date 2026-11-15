//go:build browsertest

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

// TestAdminLogsBrowser exercises the shipped admin HTML and modules, not a
// synthetic component mount. All APIs are local fakes; no speaker is contacted.
func TestAdminLogsBrowser(t *testing.T) {
	server := NewServer(nil, nil, "http://localhost:8001", false, false, false)
	server.SetPlayerEnabled(false)
	var polls atomic.Int64
	r := chi.NewRouter()
	r.Get("/admin", server.HandleAdmin)
	r.Get("/web/*", server.HandleWeb())
	r.Get("/api/setup/settings", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"discovery_enabled":false}`))
	})
	r.Get("/api/setup/devices", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	r.Get("/api/setup/logs", func(w http.ResponseWriter, _ *http.Request) {
		polls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entries":   []map[string]string{{"time": "2026-10-26T12:00:00Z", "message": `<img src=x onerror="window.xss=1"> DEVICEID01`}},
			"nextSince": 1, "dropped": 0,
		})
	})
	r.Get("/api/announcements", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"announcements":[]}`))
	})
	r.Get("/api/setup/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	ts := httptest.NewServer(r)
	defer ts.Close()

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true), chromedp.Flag("no-sandbox", true))...)
	defer cancelAlloc()
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	ctx, cancelTimeout := context.WithTimeout(ctx, 15*time.Second)
	defer cancelTimeout()

	if err := chromedp.Do(ctx,
		chromedp.Navigate(ts.URL+"/admin#tab-logs"),
		chromedp.Poll[chromedp.Void](`document.querySelector('#tab-logs [data-admin-ui="preact-logs"]') && document.querySelector('#logs-view')?.textContent.includes('DEVICEID01')`),
	); err != nil {
		t.Fatal(err)
	}
	var text string
	var injected bool
	if err := chromedp.Do(ctx,
		adminInto(chromedp.Text(chromedp.CSS(`#logs-view`)), &text),
		adminInto(chromedp.Evaluate[bool](`!!document.querySelector('#logs-view img') || !!window.xss`), &injected),
	); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `<img src=x onerror="window.xss=1"> DEVICEID01`) {
		t.Errorf("log payload not rendered as text: %q", text)
	}
	if injected {
		t.Error("log payload was parsed as markup or executed")
	}
	if err := chromedp.Do(ctx,
		chromedp.SendKeys(chromedp.CSS(`#logs-filter`), "no-match"),
		chromedp.Poll[chromedp.Void](`document.querySelector('#logs-view')?.textContent === ''`),
		chromedp.Click(chromedp.CSS(`button[onclick*="tab-settings"]`)),
		chromedp.Poll[chromedp.Void](`!document.querySelector('#tab-logs')?.classList.contains('active')`),
	); err != nil {
		t.Fatal(err)
	}
	prior := polls.Load()
	time.Sleep(1800 * time.Millisecond)
	if got := polls.Load(); got != prior {
		t.Errorf("logs kept polling outside their tab: before=%d after=%d", prior, got)
	}
	if err := chromedp.Do(ctx,
		chromedp.Click(chromedp.CSS(`button[onclick*="tab-logs"]`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#logs-filter')?.value === 'no-match'`),
	); err != nil {
		t.Fatal(err)
	}
	if got := polls.Load(); got <= prior {
		t.Errorf("reopening Logs did not request a fresh snapshot: before=%d after=%d", prior, got)
	}
}

func adminInto[T any](action chromedp.Action[T], dest *T) chromedp.Action[chromedp.Void] {
	return func(ctx context.Context, target *chromedp.Target) (chromedp.Void, error) {
		value, err := action(ctx, target)
		if err == nil {
			*dest = value
		}
		return chromedp.Void{}, err
	}
}
