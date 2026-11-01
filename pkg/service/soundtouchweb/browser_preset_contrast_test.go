//go:build browsertest

package soundtouchweb

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// selectedControlFixture renders the real components, so a change to their
// markup can't leave these tests checking a tile the player no longer draws.
// Transitions are off: a colour in mid-transition would still read as the old
// value and let a broken rule pass.
const selectedControlFixture = `
import { h, render } from 'preact';
import { Presets } from '/app/static/js/components/Presets.js';
import { Sources } from '/app/static/js/components/Sources.js';

const style = document.createElement('style');
style.textContent = '* { transition: none !important; }';
document.head.append(style);

const item = (location, name) => ({ Source: 'TUNEIN', Location: location, ItemName: name });
const status = {
  revision: 1,
  presets: { Preset: [{ ID: 1, ContentItem: item('/a', 'Selected') }, { ID: 2, ContentItem: item('/b', 'Other') }] },
  nowPlaying: { Source: 'TUNEIN', SourceAccount: '', ContentItem: item('/a', 'Selected') },
  sources: { SourceItem: [{ Source: 'TUNEIN', SourceAccount: '', Status: 'READY' }, { Source: 'AUX', SourceAccount: '', Status: 'READY' }] },
};
const root = document.getElementById('fixture');
window.setBusy = busy => render(h('div', {},
  h(Presets, { deviceId: 'speaker', status, commandBusy: busy, onSelect() {} }),
  h(Sources, { deviceId: 'speaker', status })), root);
window.setBusy(false);

const channels = color => color.match(/[\d.]+/g).map(Number);
const over = (top, alpha, under) => top.slice(0, 3).map((value, i) => value * alpha + under[i] * (1 - alpha));
const luminance = rgb => {
  const [r, g, b] = rgb.map(value => {
    const c = value / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
// WCAG contrast of text against its filled box, as it reaches the screen: both
// are composited over the page at the box's own opacity.
window.contrast = (boxSelector, textSelector) => {
  const box = document.querySelector(boxSelector);
  const text = textSelector ? box.querySelector(textSelector) : box;
  const page = channels(getComputedStyle(document.body).backgroundColor);
  const alpha = Number(getComputedStyle(box).opacity);
  const background = over(channels(getComputedStyle(box).backgroundColor), alpha, page);
  const foreground = over(channels(getComputedStyle(text).color), alpha, page);
  const [light, dark] = [luminance(background), luminance(foreground)].sort((a, b) => b - a);
  return (light + 0.05) / (dark + 0.05);
};
// Resolves a colour variable to the form getComputedStyle reports.
window.resolved = name => {
  const probe = document.createElement('i');
  probe.style.color = 'var(' + name + ')';
  document.body.append(probe);
  const value = getComputedStyle(probe).color;
  probe.remove();
  return value;
};
window.center = selector => {
  const element = document.querySelector(selector);
  element.scrollIntoView({ block: 'center' });
  const rect = element.getBoundingClientRect();
  return [rect.left + rect.width / 2, rect.top + rect.height / 2];
};`

func selectedControlPage(t *testing.T) context.Context {
	t.Helper()

	server := newMetadataFixtureServer(t, selectedControlFixture)
	ctx := newHeadlessChromeContext(t)

	if err := chromedp.Do(ctx, chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`.preset-slot.active`)),
		chromedp.WaitVisible(chromedp.CSS(`.source-btn.active`))); err != nil {
		t.Fatalf("selected controls did not render: %v", err)
	}

	return ctx
}

func emulateColorScheme(t *testing.T, ctx context.Context, scheme string) {
	t.Helper()

	if _, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{
		Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}},
	}); err != nil {
		t.Fatal(err)
	}
}

func hoverOver(t *testing.T, ctx context.Context, selector string) {
	t.Helper()

	var center []float64
	if err := chromedp.Do(ctx,
		evalInto(`window.center(`+"`"+selector+"`"+`)`, &center),
	); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx,
		chromedp.MouseEvent(input.DispatchMouseEventTypeMouseMoved, center[0], center[1]),
		chromedp.Poll[chromedp.Void](`document.querySelector(`+"`"+selector+"`"+`).matches(':hover')`),
	); err != nil {
		t.Fatalf("pointer did not reach %s: %v", selector, err)
	}
}

func readableExpr(box, text string) string {
	return `window.contrast(` + "`" + box + "`, `" + text + "`" + `) >= ` + "4.5"
}

func TestSelectedControlsKeepContrastUnderHover(t *testing.T) {
	ctx := selectedControlPage(t)

	for _, scheme := range []string{"light", "dark"} {
		t.Run(scheme, func(t *testing.T) {
			emulateColorScheme(t, ctx, scheme)

			const other = `.preset-slot:not(.active):not(.empty):not(:disabled)`
			hoverOver(t, ctx, other)
			if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](
				`getComputedStyle(document.querySelector(`+"`"+other+"`"+`)).backgroundColor === window.resolved('--bg')`,
			)); err != nil {
				t.Fatalf("unselected preset lost its hover background: %v", err)
			}

			hoverOver(t, ctx, `.preset-slot.active`)
			if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](
				readableExpr(`.preset-slot.active`, `.preset-name`)+
					` && getComputedStyle(document.querySelector('.preset-slot.active')).boxShadow !== 'none'`,
			)); err != nil {
				t.Fatalf("selected preset lost contrast or its shadow under hover: %v", err)
			}

			hoverOver(t, ctx, `.source-btn.active`)
			if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](
				`window.contrast('.source-btn.active') >= 4.5`,
			)); err != nil {
				t.Fatalf("active source pill lost contrast under hover: %v", err)
			}
		})
	}
}

func TestSelectedPresetStaysReadableWhileBusy(t *testing.T) {
	ctx := selectedControlPage(t)

	for _, scheme := range []string{"light", "dark"} {
		t.Run(scheme, func(t *testing.T) {
			emulateColorScheme(t, ctx, scheme)

			if err := chromedp.Do(ctx,
				chromedp.Evaluate[chromedp.Void](`window.setBusy(true)`),
				chromedp.Poll[chromedp.Void](`document.querySelector('.preset-slot.active').disabled && `+
					readableExpr(`.preset-slot.active`, `.preset-name`)),
				chromedp.Evaluate[chromedp.Void](`window.setBusy(false)`),
				chromedp.Poll[chromedp.Void](`!document.querySelector('.preset-slot.active').disabled`),
			); err != nil {
				t.Fatalf("selected preset is unreadable while a command runs: %v", err)
			}
		})
	}
}
