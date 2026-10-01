//go:build browsertest

package soundtouchweb

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

const groupVolumeFixtureScript = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Controls } from '/app/static/js/components/Controls.js';

function projectedDevice(revision, volume) {
  return {
    status: {
      epoch: 1,
      revision,
      nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' },
      volume: { ActualVolume: volume, MuteEnabled: false },
    },
    zone: {
      isStandalone: false,
      masterControlId: 'master',
      volume,
      members: [
        { controlId: 'master', hwId: 'MASTER', actualVolume: volume, available: true },
        { controlId: 'member', hwId: 'MEMBER', actualVolume: volume - 10, available: true },
      ],
    },
  };
}

function Fixture() {
  const [projection, setProjection] = useState({ revision: 1, volume: 20 });
  window.publishProjection = (revision, volume) => setProjection({ revision, volume });
  return h('div', {},
    h(Controls, {
      deviceId: 'master',
      device: projectedDevice(projection.revision, projection.volume),
    }),
    h('output', { id: 'projection-revision' }, projection.revision),
  );
}

render(h(Fixture), document.getElementById('fixture'));
`

const memberVolumeFixtureScript = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { ZoneMemberVolumeControl } from '/app/static/js/components/ZoneMemberVolumeControl.js';

function Fixture() {
  const [projection, setProjection] = useState({ revision: 1, volume: 20 });
  window.publishProjection = (revision, volume) => setProjection({ revision, volume });
  return h('div', {},
    h(ZoneMemberVolumeControl, {
      zoneMasterId: 'master',
      memberId: 'member',
      topologyKey: 'stable-topology',
      projectionKey: 'projection-' + projection.revision,
      ariaLabel: 'Member volume',
      available: true,
      volume: projection.volume,
    }),
    h('output', { id: 'projection-revision' }, projection.revision),
  );
}

render(h(Fixture), document.getElementById('fixture'));
`

func TestZoneVolumeDelayedResultCannotOverwriteNewerProjection(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		route      string
		selector   string
		response   string
		finalValue string
	}{
		{
			name:       "group",
			fixture:    groupVolumeFixtureScript,
			route:      "/api/control/devices/master/zone/volume/30",
			selector:   `.volume-slider[aria-label="Group volume"]`,
			response:   `{"success":true,"data":{"requested":30,"members":[{"controlId":"master","actual":30},{"controlId":"member","actual":20}]}}`,
			finalValue: "50",
		},
		{
			name:       "member",
			fixture:    memberVolumeFixtureScript,
			route:      "/api/control/devices/master/zone/member/member/volume/30",
			selector:   `.zone-member-volume-slider[aria-label="Member volume"]`,
			response:   `{"success":true,"data":{"requested":30,"controlId":"member","members":[{"controlId":"member","actual":30}]}}`,
			finalValue: "50",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })

			var writes atomic.Int32
			server := newPlayerFixtureServer(t, test.fixture, func(r chi.Router) {
				r.Post(test.route, func(w http.ResponseWriter, _ *http.Request) {
					writes.Add(1)
					started <- struct{}{}
					<-release
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(test.response))
				})
			})

			ctx := newHeadlessChromeContext(t)
			if err := chromedp.Run(ctx,
				chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(test.selector, chromedp.ByQuery),
				chromedp.Evaluate(fmt.Sprintf(`(() => {
					const slider = document.querySelector(%q);
					slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
					slider.value = '30';
					slider.dispatchEvent(new Event('input', { bubbles: true }));
				})()`, test.selector), nil),
			); err != nil {
				t.Fatalf("start intermediate volume request: %v", err)
			}

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("intermediate volume request did not start")
			}

			if err := chromedp.Run(ctx,
				chromedp.Evaluate(`window.publishProjection(2, 50)`, nil),
				chromedp.Poll(`document.querySelector('#projection-revision').textContent === '2'`, nil),
				chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q).dispatchEvent(
					new PointerEvent('pointerup', { bubbles: true }))`, test.selector), nil),
			); err != nil {
				t.Fatalf("publish newer projection and finish drag: %v", err)
			}

			releaseOnce.Do(func() { close(release) })
			if err := chromedp.Run(ctx,
				chromedp.Poll(`document.querySelector('.volume-row[aria-busy="true"], .zone-member-volume-control[aria-busy="true"]') === null`, nil),
				chromedp.Poll(fmt.Sprintf(`document.querySelector(%q).value === %q`,
					test.selector, test.finalValue), nil),
			); err != nil {
				t.Fatalf("wait for stale response fencing: %v", err)
			}

			if got := writes.Load(); got != 1 {
				t.Errorf("volume writes = %d, want one intermediate write and no duplicate final write", got)
			}
		})
	}
}

func TestZoneVolumeUntouchedBlurDoesNotWriteAndFinalFailureSurvivesBlur(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		route       string
		selector    string
		failureText string
	}{
		{
			name:        "group",
			fixture:     groupVolumeFixtureScript,
			route:       "/api/control/devices/master/zone/volume/30",
			selector:    `.volume-slider[aria-label="Group volume"]`,
			failureText: "write rejected",
		},
		{
			name:        "member",
			fixture:     memberVolumeFixtureScript,
			route:       "/api/control/devices/master/zone/member/member/volume/30",
			selector:    `.zone-member-volume-slider[aria-label="Member volume"]`,
			failureText: "write rejected",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })

			var writes atomic.Int32
			server := newPlayerFixtureServer(t, test.fixture, func(r chi.Router) {
				r.Post(test.route, func(w http.ResponseWriter, _ *http.Request) {
					writes.Add(1)
					started <- struct{}{}
					<-release
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadGateway)
					_, _ = w.Write([]byte(`{"success":false,"error":"write rejected"}`))
				})
			})

			ctx := newHeadlessChromeContext(t)
			if err := chromedp.Run(ctx,
				chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(test.selector, chromedp.ByQuery),
				chromedp.Focus(test.selector, chromedp.ByQuery),
				chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q).blur()`, test.selector), nil),
				chromedp.Sleep(250*time.Millisecond),
			); err != nil {
				t.Fatalf("focus and blur untouched slider: %v", err)
			}
			if got := writes.Load(); got != 0 {
				t.Fatalf("untouched focus/blur wrote volume %d times, want zero", got)
			}

			if err := chromedp.Run(ctx,
				chromedp.Evaluate(fmt.Sprintf(`(() => {
					const slider = document.querySelector(%q);
					slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
					slider.value = '30';
					slider.dispatchEvent(new Event('input', { bubbles: true }));
				})()`, test.selector), nil),
			); err != nil {
				t.Fatalf("start failed volume request: %v", err)
			}

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("failed volume request did not start")
			}
			if err := chromedp.Run(ctx, chromedp.Poll(
				`document.querySelector('.volume-row[aria-busy="true"], .zone-member-volume-control[aria-busy="true"]') !== null`,
				nil,
			)); err != nil {
				t.Fatalf("wait for failed request to become active: %v", err)
			}
			releaseOnce.Do(func() { close(release) })

			if err := chromedp.Run(ctx,
				chromedp.Poll(`document.querySelector('.volume-row[aria-busy="true"], .zone-member-volume-control[aria-busy="true"]') === null`, nil),
				chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q).dispatchEvent(
					new PointerEvent('pointerup', { bubbles: true }))`, test.selector), nil),
				chromedp.Poll(fmt.Sprintf(`document.body.textContent.includes(%q)`, test.failureText), nil),
				chromedp.Evaluate(fmt.Sprintf(`(() => {
					const slider = document.querySelector(%q);
					slider.focus();
					slider.blur();
				})()`, test.selector), nil),
				chromedp.Sleep(250*time.Millisecond),
			); err != nil {
				t.Fatalf("surface and retain final failure: %v", err)
			}

			var failureVisible bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(
				fmt.Sprintf(`document.body.textContent.includes(%q)`, test.failureText),
				&failureVisible,
			)); err != nil {
				t.Fatalf("read retained failure: %v", err)
			}
			if !failureVisible {
				t.Error("forced-final failure disappeared after blur")
			}
			if got := writes.Load(); got != 1 {
				t.Errorf("failed volume writes = %d, want one intermediate write and no duplicate final write", got)
			}
		})
	}
}

func TestZoneMemberVolumeAvailabilityPresentation(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { ZoneMemberVolumeControl } from '/app/static/js/components/ZoneMemberVolumeControl.js';

render(h('div', {},
  h(ZoneMemberVolumeControl, {
    zoneMasterId: 'master', memberId: 'unavailable', topologyKey: 'topology',
    projectionKey: 'projection', ariaLabel: 'Unavailable member volume',
    available: false, volume: 33,
  }),
  h(ZoneMemberVolumeControl, {
    zoneMasterId: 'master', memberId: 'unknown', topologyKey: 'topology',
    projectionKey: 'projection', ariaLabel: 'Unknown member volume',
    available: true, volume: null,
  }),
), document.getElementById('fixture'));
`

	server := newPlayerFixtureServer(t, fixture, func(chi.Router) {})
	ctx := newHeadlessChromeContext(t)

	var presentation map[string]bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.zone-member-volume-control`, chromedp.ByQuery),
		chromedp.Evaluate(`({
			unavailableIsRange: document.querySelector('[aria-label="Unavailable member volume"]')?.matches('input[type="range"]') === true,
			unavailableDisabled: document.querySelector('[aria-label="Unavailable member volume"]')?.disabled === true,
			unavailableExplained: document.body.textContent.includes('Volume unavailable.'),
			unknownHasNoRange: document.querySelector('[aria-label="Unknown member volume"] input[type="range"]') === null,
			unknownHasPlaceholder: document.querySelector('[aria-label="Unknown member volume"] .zone-member-volume-slider-unknown') !== null,
			unknownExplained: document.body.textContent.includes('Volume readback unknown.'),
		})`, &presentation),
	); err != nil {
		t.Fatalf("inspect member volume availability presentation: %v", err)
	}

	for name, passed := range presentation {
		if !passed {
			t.Errorf("presentation check %s failed", name)
		}
	}
}
