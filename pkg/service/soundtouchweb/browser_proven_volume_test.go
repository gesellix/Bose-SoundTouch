//go:build browsertest

package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

func TestProvenGroupCardVolumeCoalescesAndKeepsNavigation(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceList } from '/app/static/js/components/DeviceList.js';
localStorage.setItem('aftertouch_device_sort', 'ip');
function Fixture() {
  const [state, setState] = useState({ revision: 1, master: 20, member: 15 });
  window.publishCardVolume = (master, member) => setState(previous => ({ revision: previous.revision + 1, master, member }));
  const device = { info: { device_id: 'MASTER', name: 'Kitchen group', type: 'SoundTouch 10', ip_address: '192.0.2.10' },
    status: { epoch: 1, revision: state.revision, connectivity: 'online', volume: { ActualVolume: state.master },
      nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' } },
    zone: { masterControlId: 'master', masterDeviceId: 'MASTER', volume: state.master, memberCount: 2,
      physicalMemberCount: 2, availableMemberCount: 2, degraded: false, members: [
        { controlId: 'master', hwId: 'MASTER', name: 'Kitchen', actualVolume: state.master, available: true },
        { controlId: 'member', hwId: 'MEMBER', name: 'Room', actualVolume: state.member, available: true } ] } };
  return h(DeviceList, { devices: { master: device, member: {
      info: { device_id: 'MEMBER', name: 'Room', type: 'SoundTouch 20' },
      status: { epoch: 1, revision: state.revision, volume: { ActualVolume: state.member } } } }, isDiscovering: false,
    onSelect: id => { window.openedDetail = id; }, onDiscover: () => {} });
}
render(h(Fixture), document.getElementById('fixture'));
`
	var writes atomic.Int32
	var active atomic.Int32
	var overlap atomic.Bool
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	master, member := 20, 15
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/master/zone/volume/{volume}", func(w http.ResponseWriter, request *http.Request) {
			if active.Add(1) != 1 {
				overlap.Store(true)
			}
			defer active.Add(-1)
			count := writes.Add(1)
			if count == 1 {
				started <- struct{}{}
				<-release
			}
			level, err := strconv.Atoi(chi.URLParam(request, "volume"))
			if err != nil {
				http.Error(w, "bad volume", http.StatusBadRequest)
				return
			}
			delta := level - master
			master, member = level, member+delta
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
				"requested": level, "partial": false, "members": []map[string]any{
					{"controlId": "master", "target": master, "actual": master, "status": "confirmed"},
					{"controlId": "member", "target": member, "actual": member, "status": "confirmed"},
				},
			}})
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`.zone-volume-slider`)),
		chromedp.Evaluate[chromedp.Void](`(() => {
  const slider = document.querySelector('.zone-volume-slider');
  if (slider.closest('button')) throw new Error('Volume slider is nested in a navigation button');
  slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
  slider.value = '30'; slider.dispatchEvent(new Event('input', { bubbles: true }));
})()`),
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("card volume request did not start")
	}
	if err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`(() => {
  const slider = document.querySelector('.zone-volume-slider');
  for (const value of [5, 45, 23]) {
    slider.value = value; slider.dispatchEvent(new Event('input', { bubbles: true }));
  }
  slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
  slider.dispatchEvent(new Event('change', { bubbles: true }));
  slider.dispatchEvent(new Event('blur', { bubbles: true }));
})()`),
	); err != nil {
		t.Fatal(err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := chromedp.Do(ctx,
		chromedp.Poll[chromedp.Void](`document.querySelector('.zone-volume-slider').value === '23' &&
  document.querySelector('[aria-busy="true"]') === null`),
		chromedp.Evaluate[chromedp.Void](`window.publishCardVolume(23, 18)`),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('.zone-card-open').click()`),
		chromedp.Poll[chromedp.Void](`window.openedDetail === 'master' &&
  document.querySelector('.device-ip').textContent.includes('192.0.2.10')`),
	); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 2 || overlap.Load() {
		t.Fatalf("card requests = %d, overlap = %v; want first + latest final serialized", writes.Load(), overlap.Load())
	}
}

func TestProvenGroupVolumeWarningRequiresMatchingAuthoritativeValues(t *testing.T) {
	for _, outcome := range []string{"unverified", "failed"} {
		t.Run(outcome, func(t *testing.T) {
			var writes atomic.Int32
			server := newPlayerFixtureServer(t, groupVolumeFixtureScript, func(r chi.Router) {
				r.Post("/api/control/devices/master/zone/volume/30", func(w http.ResponseWriter, _ *http.Request) {
					writes.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
						"requested": 30, "partial": true, "members": []map[string]any{
							{"controlId": "master", "name": "Kitchen", "target": 30, "actual": 20, "status": outcome, "error": "Pending outcome"},
							{"controlId": "member", "target": 20, "actual": 20, "status": "confirmed"},
						},
					}})
				})
			})
			ctx := newHeadlessChromeContext(t)
			if err := chromedp.Do(ctx,
				chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(chromedp.CSS(`.volume-slider[aria-label="Group volume"]`)),
				chromedp.Evaluate[chromedp.Void](`(() => {
  const slider = document.querySelector('.volume-slider');
  slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
  slider.value = '30'; slider.dispatchEvent(new Event('input', { bubbles: true }));
  slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
})()`),
				chromedp.Poll[chromedp.Void](`document.querySelector('.volume-control-failure') !== null`),
				chromedp.Evaluate[chromedp.Void](`window.publishProjection(2, 20)`),
				chromedp.Sleep(100*time.Millisecond),
				chromedp.Poll[chromedp.Void](`document.querySelector('.volume-control-failure') !== null`),
				chromedp.Evaluate[chromedp.Void](`window.publishProjection(3, 30)`),
			); err != nil {
				t.Fatal(err)
			}
			condition := `document.querySelector('.volume-control-failure') === null`
			if outcome == "failed" {
				condition = `document.querySelector('.volume-control-failure') !== null`
			}
			if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](condition)); err != nil {
				t.Fatalf("%s warning did not follow authoritative confirmation: %v", outcome, err)
			}
			if writes.Load() != 1 {
				t.Fatalf("confirmation replayed write: %d", writes.Load())
			}
		})
	}
}

func TestProvenPartialGroupKeepsFailedLoudestMember(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Controls } from '/app/static/js/components/Controls.js';
import { DeviceList } from '/app/static/js/components/DeviceList.js';
function Fixture() {
  const [revision, setRevision] = useState(1);
  window.publishAgain = () => setRevision(value => value + 1);
  const device = { info: { device_id: 'MASTER', name: 'Group' },
    status: { epoch: 1, revision, volume: { ActualVolume: 90 } },
    zone: { masterControlId: 'master', masterDeviceId: 'MASTER', volume: 90,
      memberCount: 2, availableMemberCount: 2, members: [
        { controlId: 'master', hwId: 'MASTER', actualVolume: 90 },
        { controlId: 'member', hwId: 'MEMBER', actualVolume: 20 } ] } };
  const devices = { master: device, member: { info: { device_id: 'MEMBER' },
    status: { epoch: 1, revision, volume: { ActualVolume: 20 } } } };
  return h('div', {},
    h('div', { id: 'detail' }, h(Controls, { deviceId: 'master', device, devices })),
    h('div', { id: 'card' }, h(DeviceList, { devices, onSelect: () => {} })));
}

render(h(Fixture), document.getElementById('fixture'));
`
	var writes atomic.Int32
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/master/zone/volume/100", func(w http.ResponseWriter, _ *http.Request) {
			writes.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"requested":100,"partial":true,"members":[
{"controlId":"master","name":"Loudest","target":100,"status":"failed","error":"Refused"},
{"controlId":"member","target":30,"actual":30,"status":"confirmed"}]}}`))
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#detail .volume-slider`)),
		chromedp.WaitVisible(chromedp.CSS(`#card .zone-volume-slider`)),
		chromedp.Evaluate[chromedp.Void](`(() => {
  for (const selector of ['#detail .volume-slider', '#card .zone-volume-slider']) {
    const slider = document.querySelector(selector);
    slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
    slider.value = '100'; slider.dispatchEvent(new Event('input', { bubbles: true }));
    slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
  }
})()`),
		chromedp.Poll[chromedp.Void](`document.querySelector('#detail .volume-control-failure')?.textContent.includes('failed') &&
  document.querySelector('#card .zone-volume-failure')?.textContent.includes('failed')`),
		chromedp.Poll[chromedp.Void](`document.querySelector('#detail .volume-slider').value === '90' &&
  document.querySelector('#card .zone-volume-slider').value === '90'`),
		chromedp.Evaluate[chromedp.Void](`window.publishAgain()`),
		chromedp.Sleep(100*time.Millisecond),
		chromedp.Poll[chromedp.Void](`document.querySelector('#detail .volume-slider').value === '90' &&
  document.querySelector('#card .zone-volume-slider').value === '90'`),
	); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 2 {
		t.Fatalf("partial readback replayed writes: %d", writes.Load())
	}
}

func TestProvenPartialProgressKeepsNewerMemberActual(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Controls } from '/app/static/js/components/Controls.js';
import { DeviceList } from '/app/static/js/components/DeviceList.js';
function Fixture() {
  const [state, setState] = useState({ revision: 1, volume: 20 });
  window.publishPartialProgress = () => setState({ revision: 2, volume: 40 });
  const device = { info: { device_id: 'MASTER', name: 'Group' },
    status: { epoch: 1, revision: state.revision, volume: { ActualVolume: state.volume } },
    zone: { masterControlId: 'master', masterDeviceId: 'MASTER', volume: state.volume,
      memberCount: 2, availableMemberCount: 2, members: [
        { controlId: 'master', hwId: 'MASTER', actualVolume: state.volume },
        { controlId: 'child', hwId: 'CHILD', actualVolume: 0 } ] } };
  const devices = { master: device, child: { info: { device_id: 'CHILD' },
    status: { epoch: 1, revision: 1, volume: { ActualVolume: 0 } } } };
  return h('div', {},
    h('div', { id: 'detail' }, h(Controls, { deviceId: 'master', device, devices })),
    h('div', { id: 'card' }, h(DeviceList, { devices, onSelect: () => {} })));
}
render(h(Fixture), document.getElementById('fixture'));
`
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/master/zone/volume/40", func(w http.ResponseWriter, _ *http.Request) {
			started <- struct{}{}
			<-release
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"requested":40,"partial":true,"members":[
{"controlId":"master","target":40,"actual":35,"status":"unverified"},
{"controlId":"child","target":20,"actual":15,"status":"unverified"}]}}`))
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(chromedp.CSS(`#card .zone-volume-slider`)),
		chromedp.Evaluate[chromedp.Void](`(() => {
  for (const selector of ['#detail .volume-slider', '#card .zone-volume-slider']) {
    const slider = document.querySelector(selector);
    slider.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
    slider.value = '40'; slider.dispatchEvent(new Event('input', { bubbles: true }));
    slider.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
  }
})()`),
	); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("partial progress requests did not start")
		}
	}
	if err := chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.publishPartialProgress()`),
		chromedp.Poll[chromedp.Void](`document.querySelector('#detail .volume-slider').value === '40'`)); err != nil {
		t.Fatal(err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := chromedp.Do(ctx,
		chromedp.Poll[chromedp.Void](`document.querySelector('#detail .volume-control-failure')?.textContent.includes('unverified') &&
  document.querySelector('#card .zone-volume-failure')?.textContent.includes('unverified')`),
		chromedp.Poll[chromedp.Void](`document.querySelector('#detail .volume-slider').value === '40' &&
  document.querySelector('#card .zone-volume-slider').value === '40'`),
	); err != nil {
		t.Fatal(err)
	}
}
