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
  const device = projectedDevice(projection.revision, projection.volume);
  const devices = { master: device, member: { info: { device_id: 'MEMBER' },
    status: { epoch: 1, revision: projection.revision, volume: { ActualVolume: projection.volume - 10 } } } };
  return h('div', {},
    h(Controls, {
      deviceId: 'master',
      device, devices,
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

func TestZoneVolumeOptimisticSharedDeltaIncludesLogicalStereoMember(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { DeviceDetail } from '/app/static/js/app.js';
const members = [
  { controlId: 'speaker', hwId: 'MASTER', name: 'Kitchen', kind: 'device',
    deviceIds: ['MASTER'], actualVolume: 95, available: true },
  { controlId: 'pair', hwId: 'RIGHT', name: 'Living Room', kind: 'stereoPair',
    deviceIds: ['LEFT', 'RIGHT'], actualVolume: 70, available: true,
    physicalMembers: [
      { deviceId: 'LEFT', role: 'LEFT', available: true },
      { deviceId: 'RIGHT', role: 'RIGHT', available: true },
    ] },
];
const devices = {
  speaker: {
    info: { name: 'Kitchen' },
    status: { epoch: 1, revision: 1, volume: { ActualVolume: 95, MuteEnabled: false } },
    zone: { isStandalone: false, masterControlId: 'speaker', masterDeviceId: 'MASTER',
      volume: 95, memberCount: 2, physicalMemberCount: 3, members },
  },
  pair: { info: { name: 'Living Room' }, status: { epoch: 1, revision: 1 } },
};
render(h(DeviceDetail, { deviceId: 'speaker', devices, onBack: () => {} }),
  document.getElementById('fixture'));
`
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var writes atomic.Int32
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		registerDeviceDetailSideRoutes(r)
		r.Post("/api/control/devices/speaker/zone/volume/100", func(w http.ResponseWriter, _ *http.Request) {
			writes.Add(1)
			started <- struct{}{}
			<-release
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"requested":100,"partial":false,"members":[{"controlId":"speaker","target":100,"actual":100,"status":"confirmed"},{"controlId":"pair","target":75,"actual":75,"status":"confirmed"}]}}`))
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`.volume-slider[aria-label="Group volume"]`)),
		chromedp.WaitVisible(chromedp.CSS(`.zone-member-volume-slider[aria-label="Living Room volume"]`)),
		chromedp.Evaluate[chromedp.Void](`(() => {
			const slider = document.querySelector('.volume-slider[aria-label="Group volume"]');
			slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
			slider.value = '100';
			slider.dispatchEvent(new Event('input', { bubbles: true }));
		})()`),
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("group write did not start")
	}
	if err := chromedp.Do(ctx,
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-slider[aria-label="Group volume"]').value === '100' &&
			document.querySelector('.zone-member-volume-slider[aria-label="Kitchen volume"]').value === '100' &&
			document.querySelector('.zone-member-volume-slider[aria-label="Living Room volume"]').value === '75' &&
			document.querySelectorAll('.zone-member-volume-slider').length === 2`),
	); err != nil {
		t.Fatalf("optimistic logical member delta before HTTP response: %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := chromedp.Do(ctx,
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-row[aria-busy="true"]') === null`),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('.volume-slider[aria-label="Group volume"]')
			.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }))`),
		chromedp.Sleep(250*time.Millisecond),
	); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 1 {
		t.Fatalf("writes = %d, want one write including final value", writes.Load())
	}
}

func TestDirectVolumeSliderPreservesGestureAndIdleEvents(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Controls } from '/app/static/js/components/Controls.js';
function Fixture() {
  const [volume, setVolume] = useState(20);
  window.publishVolume = setVolume;
  return h(Controls, { deviceId: 'speaker', status: { volume: { ActualVolume: volume } } });
}
render(h(Fixture), document.getElementById('fixture'));
`
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var writes atomic.Int32
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/volume/40", func(w http.ResponseWriter, _ *http.Request) {
			writes.Add(1)
			started <- struct{}{}
			<-release
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"message":"Volume set to 40"}}`))
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(".volume-slider")),
		chromedp.Evaluate[chromedp.Void](`(() => {
			const slider = document.querySelector('.volume-slider');
			slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
			slider.value = '40';
			slider.dispatchEvent(new Event('input', { bubbles: true }));
		})()`),
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("direct volume write did not start")
	}
	if err := chromedp.Do(ctx,
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-slider').value === '40'`),
		chromedp.Evaluate[chromedp.Void]("window.publishVolume(22)"),
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-slider').value === '40'`),
	); err != nil {
		t.Fatalf("speaker event moved the active direct slider: %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`(() => {
			const slider = document.querySelector('.volume-slider');
			slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
			slider.dispatchEvent(new Event('change', { bubbles: true }));
			slider.dispatchEvent(new Event('blur', { bubbles: true }));
		})()`),
		chromedp.Sleep(250*time.Millisecond),
		chromedp.Evaluate[chromedp.Void]("window.publishVolume(21)"),
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-slider').value === '21'`),
	); err != nil {
		t.Fatal(err)
	}
	var unchanged bool
	if err := chromedp.Do(ctx, evalInto(`document.querySelector('.volume-slider').value === '21' &&
		document.querySelector('.volume-control-failure') === null &&
		document.querySelector('.volume-row').getAttribute('aria-busy') === null`, &unchanged)); err != nil {
		t.Fatal(err)
	}
	if !unchanged || writes.Load() != 1 {
		t.Fatalf("direct response changed slider behavior: unchanged=%v writes=%d", unchanged, writes.Load())
	}
}

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
			if err := chromedp.Do(ctx,
				chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(chromedp.CSS(test.selector)),
				chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`(() => {
					const slider = document.querySelector(%q);
					slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
					slider.value = '30';
					slider.dispatchEvent(new Event('input', { bubbles: true }));
				})()`, test.selector)),
			); err != nil {
				t.Fatalf("start intermediate volume request: %v", err)
			}

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("intermediate volume request did not start")
			}

			if err := chromedp.Do(ctx,
				chromedp.Poll[chromedp.Void](fmt.Sprintf(`document.querySelector(%q).value === '30'`, test.selector)),
				chromedp.Evaluate[chromedp.Void](`window.publishProjection(2, 50)`),
				chromedp.Poll[chromedp.Void](`document.querySelector('#projection-revision').textContent === '2'`),
				chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.querySelector(%q).dispatchEvent(
					new PointerEvent('pointerup', { bubbles: true }))`, test.selector)),
			); err != nil {
				t.Fatalf("publish newer projection and finish drag: %v", err)
			}

			releaseOnce.Do(func() { close(release) })
			if err := chromedp.Do(ctx,
				chromedp.Poll[chromedp.Void](`document.querySelector('.volume-row[aria-busy="true"], .zone-member-volume-control[aria-busy="true"]') === null`),
				chromedp.Poll[chromedp.Void](fmt.Sprintf(`document.querySelector(%q).value === %q`,
					test.selector, test.finalValue)),
			); err != nil {
				t.Fatalf("wait for optimistic preview and stale response fencing: %v", err)
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
			if err := chromedp.Do(ctx,
				chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(chromedp.CSS(test.selector)),
				chromedp.Focus(chromedp.CSS(test.selector)),
				chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.querySelector(%q).blur()`, test.selector)),
				chromedp.Sleep(250*time.Millisecond),
			); err != nil {
				t.Fatalf("focus and blur untouched slider: %v", err)
			}
			if got := writes.Load(); got != 0 {
				t.Fatalf("untouched focus/blur wrote volume %d times, want zero", got)
			}

			if err := chromedp.Do(ctx,
				chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`(() => {
					const slider = document.querySelector(%q);
					slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
					slider.value = '30';
					slider.dispatchEvent(new Event('input', { bubbles: true }));
				})()`, test.selector)),
			); err != nil {
				t.Fatalf("start failed volume request: %v", err)
			}

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("failed volume request did not start")
			}
			if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](
				`document.querySelector('.volume-row[aria-busy="true"], .zone-member-volume-control[aria-busy="true"]') !== null`,
			)); err != nil {
				t.Fatalf("wait for failed request to become active: %v", err)
			}
			releaseOnce.Do(func() { close(release) })

			if err := chromedp.Do(ctx,
				chromedp.Poll[chromedp.Void](`document.querySelector('.volume-row[aria-busy="true"], .zone-member-volume-control[aria-busy="true"]') === null`),
				chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.querySelector(%q).dispatchEvent(
					new PointerEvent('pointerup', { bubbles: true }))`, test.selector)),
				chromedp.Poll[chromedp.Void](fmt.Sprintf(`document.body.textContent.includes(%q)`, test.failureText)),
				chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`(() => {
					const slider = document.querySelector(%q);
					slider.focus();
					slider.blur();
				})()`, test.selector)),
				chromedp.Sleep(250*time.Millisecond),
			); err != nil {
				t.Fatalf("surface and retain final failure: %v", err)
			}

			var failureVisible bool
			if err := chromedp.Do(ctx, evalInto(
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
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`.zone-member-volume-control`)),
		evalInto(`({
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

// TestUntouchedGroupPressDoesNotFreezeMemberSliders: pressing the group slider
// starts a preview of the member volumes. A press that changes nothing must
// drop it again, or the member sliders keep showing it over their live
// volumes until the next group interaction.
func TestUntouchedGroupPressDoesNotFreezeMemberSliders(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail } from '/app/static/js/app.js';
function devicesFor(pairVolume, revision) {
  const members = [
    { controlId: 'speaker', hwId: 'MASTER', name: 'Kitchen', kind: 'device',
      deviceIds: ['MASTER'], actualVolume: 50, available: true },
    { controlId: 'other', hwId: 'OTHER', name: 'Office', kind: 'device',
      deviceIds: ['OTHER'], actualVolume: pairVolume, available: true },
  ];
  return {
    speaker: {
      info: { name: 'Kitchen' },
      status: { epoch: 1, revision, volume: { ActualVolume: 50, MuteEnabled: false } },
      zone: { isStandalone: false, masterControlId: 'speaker', masterDeviceId: 'MASTER',
        volume: 50, memberCount: 2, physicalMemberCount: 2, members },
    },
    other: { info: { name: 'Office' }, status: { epoch: 1, revision } },
  };
}
function Fixture() {
  const [state, setState] = useState({ v: 30, r: 1 });
  window.publishOther = (v, r) => setState({ v, r });
  return h(DeviceDetail, { deviceId: 'speaker', devices: devicesFor(state.v, state.r), onBack: () => {} });
}
render(h(Fixture), document.getElementById('fixture'));
`
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		registerDeviceDetailSideRoutes(r)
	})
	ctx := newHeadlessChromeContext(t)
	var after string
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`.zone-member-volume-slider[aria-label="Office volume"]`)),
		chromedp.Evaluate[chromedp.Void](`(() => {
			const slider = document.querySelector('.volume-slider[aria-label="Group volume"]');
			slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
			slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
		})()`),
		chromedp.Evaluate[chromedp.Void](`window.publishOther(10, 2)`),
		chromedp.Sleep(1500*time.Millisecond),
		evalInto(`document.querySelector('.zone-member-volume-slider[aria-label="Office volume"]').value`, &after),
	); err != nil {
		t.Fatal(err)
	}
	if after != "10" {
		t.Errorf("stale preview masks authoritative member volume: shows %s, want 10", after)
	}
}

// TestDirectVolumeDragKeepsPointerValue proves a lagging speaker event cannot
// move a standalone/pair slider underneath an active pointer interaction.
func TestDirectVolumeDragKeepsPointerValue(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Controls } from '/app/static/js/components/Controls.js';
function Fixture() {
  const [state, setState] = useState({ volume: 20, revision: 1 });
  window.publishVolume = volume => setState(previous => ({ volume, revision: previous.revision + 1 }));
  return h(Controls, { deviceId: 'speaker', status: { epoch: 1, revision: state.revision,
    volume: { ActualVolume: state.volume } } });
}
render(h(Fixture), document.getElementById('fixture'));
`
	var writes atomic.Int32
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/volume/40", func(w http.ResponseWriter, _ *http.Request) {
			writes.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"message":"Volume set to 40"}}`))
		})
	})
	ctx := newHeadlessChromeContext(t)
	var held string
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(".volume-slider")),
		chromedp.Evaluate[chromedp.Void](`(() => {
   const slider = document.querySelector('.volume-slider');
   slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
   slider.value = '40';
   slider.dispatchEvent(new Event('input', { bubbles: true }));
  })()`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-slider').value === '40'`),
		chromedp.Evaluate[chromedp.Void](`window.publishVolume(22)`),
		chromedp.Sleep(300*time.Millisecond),
		evalInto(`document.querySelector('.volume-slider').value`, &held),
	); err != nil {
		t.Fatal(err)
	}
	if held != "40" {
		t.Fatalf("lagging speaker readback moved active slider to %s, want pointer value 40", held)
	}
	if err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`(() => {
   const slider = document.querySelector('.volume-slider');
   slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
   slider.dispatchEvent(new Event('change', { bubbles: true }));
   slider.dispatchEvent(new Event('blur', { bubbles: true }));
  })()`),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate[chromedp.Void](`window.publishVolume(23)`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-slider').value === '23'`),
	); err != nil {
		t.Fatalf("slider did not resume following speaker after interaction: %v", err)
	}
	if writes.Load() != 1 {
		t.Fatalf("direct writes = %d, want one including final value", writes.Load())
	}
}

// TestDirectVolumeFinalFailureSurvivesPassiveEvents covers the direct-mode
// lifecycle as well as the final result; untouched presses never send a write.
func TestDirectVolumeFinalFailureSurvivesPassiveEvents(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Controls } from '/app/static/js/components/Controls.js';
function Fixture() {
  const [volume, setVolume] = useState(20);
  window.publishVolume = setVolume;
  return h(Controls, { deviceId: 'speaker', status: { epoch: 1, revision: 1,
    volume: { ActualVolume: volume } } });
}
render(h(Fixture), document.getElementById('fixture'));
`
	var writes atomic.Int32
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/volume/40", func(w http.ResponseWriter, _ *http.Request) {
			writes.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"success":false,"error":"Direct volume refused"}`))
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(".volume-slider")),
		chromedp.Evaluate[chromedp.Void](`(() => {
   const slider = document.querySelector('.volume-slider');
   slider.focus(); slider.blur();
   slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
   slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
  })()`),
		chromedp.Sleep(250*time.Millisecond),
	); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 {
		t.Fatalf("untouched press sent %d direct writes", writes.Load())
	}
	if err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`(() => {
   const slider = document.querySelector('.volume-slider');
   slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
   slider.value = '40'; slider.dispatchEvent(new Event('input', { bubbles: true }));
  })()`),
		chromedp.Sleep(250*time.Millisecond),
	); err != nil {
		t.Fatal(err)
	}
	var quiet bool
	if err := chromedp.Do(ctx, evalInto(`document.querySelector('.volume-control-failure') === null`, &quiet)); err != nil {
		t.Fatal(err)
	}
	if !quiet || writes.Load() != 1 {
		t.Fatalf("intermediate direct result: quiet=%v writes=%d", quiet, writes.Load())
	}
	if err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`(() => {
   const slider = document.querySelector('.volume-slider');
   slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
   slider.dispatchEvent(new Event('change', { bubbles: true }));
   slider.focus(); slider.blur();
  })()`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.volume-control-failure')?.textContent.includes('Direct volume refused') === true`),
	); err != nil {
		t.Fatalf("forced-final error did not survive passive events: %v", err)
	}
	// Every untouched completion must release the suppressed readback while
	// preserving an already visible final failure and sending no new write.
	for index, ending := range []string{"pointerup", "pointercancel", "blur"} {
		baseline := 20 + index*2
		next := baseline + 1
		if err := chromedp.Do(ctx,
			chromedp.Evaluate[chromedp.Void](fmt.Sprintf("window.publishVolume(%d)", baseline)),
			chromedp.Poll[chromedp.Void](fmt.Sprintf("document.querySelector('.volume-slider').value === '%d'", baseline)),
			chromedp.Evaluate[chromedp.Void](`document.querySelector('.volume-slider').dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))`),
			chromedp.Evaluate[chromedp.Void](fmt.Sprintf("window.publishVolume(%d)", next)),
			chromedp.Sleep(75*time.Millisecond),
			chromedp.Evaluate[chromedp.Void](fmt.Sprintf("document.querySelector('.volume-slider').dispatchEvent(new Event('%s', { bubbles: true }))", ending)),
			chromedp.Poll[chromedp.Void](fmt.Sprintf("document.querySelector('.volume-slider').value === '%d'", next)),
			chromedp.Poll[chromedp.Void](`document.querySelector('.volume-control-failure')?.textContent.includes('Direct volume refused') === true`),
		); err != nil {
			t.Fatalf("untouched %s lost a readback or final error: %v", ending, err)
		}
	}
	if writes.Load() != 1 {
		t.Fatalf("passive completion replayed direct write: %d requests", writes.Load())
	}
}
