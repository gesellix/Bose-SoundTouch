// Command screenshots drives a headless Chrome via chromedp to capture
// PNG screenshots of the soundtouch-service web UI for documentation.
//
// It is deliberately decoupled from any speaker/service setup: callers
// are responsible for having the service reachable at --base and any
// required devices already registered. See cmd/dummy-speaker for a
// matching no-hardware backend.
//
// Manifest format (JSON):
//
//	{
//	  "shots": [
//	    {
//	      "name": "ui-settings",
//	      "path": "/web/",
//	      "click_selector": "button[onclick*=\"tab-settings\"]",
//	      "wait_selector": "#tab-settings.active",
//	      "viewport": {"width": 1280, "height": 900},
//	      "settle_ms": 250
//	    }
//	  ]
//	}
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

type viewport struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
}

type shot struct {
	Name          string   `json:"name"`
	Path          string   `json:"path"`
	ClickSelector string   `json:"click_selector,omitempty"`
	WaitSelector  string   `json:"wait_selector,omitempty"`
	Evaluate      string   `json:"evaluate,omitempty"`        // JS to run after the click (e.g. to programmatically select a device + trigger summary)
	WaitAfterEval string   `json:"wait_after_eval,omitempty"` // selector to wait for once the JS evaluation has completed
	Viewport      viewport `json:"viewport,omitempty"`
	SettleMs      int      `json:"settle_ms,omitempty"`
}

type manifest struct {
	Shots []shot `json:"shots"`
}

func main() {
	base := flag.String("base", "http://localhost:8000", "service base URL")
	manifestPath := flag.String("manifest", "scripts/screenshots/manifest.json", "path to shot manifest JSON")
	outDir := flag.String("out", "docs/images", "output directory for PNGs")
	timeoutSec := flag.Int("timeout", 30, "per-shot timeout (seconds)")
	flag.Parse()

	m, err := readManifest(*manifestPath)
	if err != nil {
		log.Fatalf("read manifest: %v", err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *outDir, err)
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("hide-scrollbars", true),
		)...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	if err := chromedp.Run(browserCtx); err != nil {
		log.Fatalf("launch browser: %v", err)
	}

	failed := 0

	for _, sh := range m.Shots {
		log.Printf("capturing %s", sh.Name)

		if err := capture(browserCtx, *base, *outDir, sh, time.Duration(*timeoutSec)*time.Second); err != nil {
			log.Printf("  failed: %v", err)

			failed++

			continue
		}

		log.Printf("  ok")
	}

	if failed > 0 {
		log.Fatalf("%d shot(s) failed", failed)
	}
}

func readManifest(path string) (*manifest, error) {
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &m, nil
}

func capture(parent context.Context, baseURL, outDir string, sh shot, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	w, h := sh.Viewport.Width, sh.Viewport.Height
	if w == 0 {
		w = 1280
	}

	if h == 0 {
		h = 900
	}

	scale := sh.Viewport.Scale
	if scale == 0 {
		scale = 2 // retina-equivalent DPR; sharper text in captured PNGs
	}

	settle := time.Duration(sh.SettleMs) * time.Millisecond
	if settle == 0 {
		settle = 200 * time.Millisecond
	}

	tabCtx, tabCancel := chromedp.NewContext(ctx)
	defer tabCancel()

	url := baseURL + sh.Path

	actions := []chromedp.Action{
		chromedp.EmulateViewport(int64(w), int64(h), chromedp.EmulateScale(scale)),
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	}

	if sh.ClickSelector != "" {
		actions = append(actions,
			chromedp.WaitVisible(sh.ClickSelector, chromedp.ByQuery),
			chromedp.Click(sh.ClickSelector, chromedp.ByQuery),
		)
	}

	if sh.WaitSelector != "" {
		actions = append(actions, chromedp.WaitVisible(sh.WaitSelector, chromedp.ByQuery))
	}

	if sh.Evaluate != "" {
		actions = append(actions, chromedp.Evaluate(sh.Evaluate, nil))
	}

	if sh.WaitAfterEval != "" {
		actions = append(actions, chromedp.WaitVisible(sh.WaitAfterEval, chromedp.ByQuery))
	}

	actions = append(actions, chromedp.Sleep(settle))

	var buf []byte

	actions = append(actions, chromedp.FullScreenshot(&buf, 100))

	if err := chromedp.Run(tabCtx, actions...); err != nil {
		return fmt.Errorf("chromedp: %w", err)
	}

	outPath := filepath.Join(outDir, sh.Name+".png")
	if err := os.WriteFile(outPath, buf, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	return nil
}
