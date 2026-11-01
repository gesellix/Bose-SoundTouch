//go:build browsertest

package soundtouchweb

import (
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// Exercise the actual cascade. Hover and active together are enough to break the
// pairing; the disable steps only cover a pointer that stays over the tile while
// a command runs (also the sticky touch-hover case).
func TestSelectedPresetKeepsContrastUnderHover(t *testing.T) {
	server := newMetadataFixtureServer(t, `
document.getElementById('fixture').innerHTML = '<button class="preset-slot" style="width:200px;height:70px" id="preset"><span class="preset-name">Station</span></button>';
window.colors = () => {
  const tile = document.getElementById('preset');
  return { background: getComputedStyle(tile).backgroundColor,
    text: getComputedStyle(tile.querySelector('.preset-name')).color,
    shadow: getComputedStyle(tile).boxShadow };
};`)
	for _, theme := range []struct{ name, background, foreground, hover string }{
		{"light", "rgb(0, 0, 0)", "rgb(255, 255, 255)", "rgb(245, 245, 245)"},
		{"dark", "rgb(224, 224, 224)", "rgb(17, 17, 17)", "rgb(17, 17, 17)"},
	} {
		t.Run(theme.name, func(t *testing.T) {
			ctx := newHeadlessChromeContext(t)
			if err := chromedp.Do(ctx, chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(chromedp.CSS(`#preset`))); err != nil {
				t.Fatal(err)
			}
			if _, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{
				Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme.name}},
			}); err != nil {
				t.Fatal(err)
			}
			if err := chromedp.Do(ctx,
				chromedp.MouseEvent(input.DispatchMouseEventTypeMouseMoved, 30, 30),
				chromedp.Poll[chromedp.Void](`window.colors().background === `+`"`+theme.hover+`"`, chromedp.WithPollingTimeout(time.Second)),
				chromedp.Evaluate[chromedp.Void](`document.getElementById('preset').classList.add('active')`),
				chromedp.Poll[chromedp.Void](`window.colors().background === "`+theme.background+`" && window.colors().text === "`+theme.foreground+`"`, chromedp.WithPollingTimeout(time.Second)),
				// Wait out the .1s transition: a fading shadow would still read as present.
				chromedp.Poll[chromedp.Void](`document.getElementById('preset').getAnimations().length === 0 && window.colors().shadow !== "none"`, chromedp.WithPollingTimeout(time.Second)),
				chromedp.Evaluate[chromedp.Void](`document.getElementById('preset').disabled = true`),
				chromedp.Poll[chromedp.Void](`window.colors().background === "`+theme.background+`"`, chromedp.WithPollingTimeout(time.Second)),
				chromedp.Evaluate[chromedp.Void](`document.getElementById('preset').disabled = false`),
				chromedp.Poll[chromedp.Void](`window.colors().background === "`+theme.background+`" && window.colors().text === "`+theme.foreground+`"`, chromedp.WithPollingTimeout(time.Second)),
			); err != nil {
				t.Fatalf("selected preset lost contrast under hover: %v", err)
			}
		})
	}
}
