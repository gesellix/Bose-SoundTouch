//go:build browsertest

package soundtouchweb

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

const recentsRefreshFixture = `
import {h,render} from 'preact';
import {Recents} from '/app/static/js/components/Recents.js';
import {api} from '/app/static/js/api.js';
window.requests=[];window.plays=[];window.writes=[];
api.recents=(id,options)=>new Promise((resolve,reject)=>requests.push({id,options,resolve,reject}));
api.play=(...args)=>writes.push(args);
let props={deviceId:'master',physicalId:'MASTER',epoch:1,revision:0,refreshToken:0};
window.show=(patch={})=>{
 props={...props,...patch};const target=props.deviceId;
 render(h(Recents,{...props,onPlay:item=>plays.push({target,name:item.ContentItem.ItemName})}),document.getElementById('fixture'));
};
window.answer=(index,name,success=true)=>requests[index].resolve({success,data:{Items:name===null?[]:[{ID:name,ContentItem:{Source:'TUNEIN',Location:'/station/'+name,ItemName:name}}]}});
window.unmount=()=>render(null,document.getElementById('fixture'));
window.foreground=visible=>{
 Object.defineProperty(document,'visibilityState',{configurable:true,value:visible?'visible':'hidden'});
 document.dispatchEvent(new Event('visibilitychange'));
};
show();
`

func TestRecentsRefreshLifecycle(t *testing.T) {
	server := newPlayerFixtureServer(t, recentsRefreshFixture, func(chi.Router) {})
	ctx := newHeadlessChromeContext(t)
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		if err := chromedp.Do(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	js := func(script string) chromedp.Action[chromedp.Void] { return chromedp.Evaluate[chromedp.Void](script) }
	wait := func(expression string) chromedp.Action[chromedp.Void] {
		return chromedp.Poll[chromedp.Void](expression)
	}
	run(navigateFixture(server.URL+"/fixture"), wait(`requests.length===1`), js(`answer(0,'Initial')`), wait(`document.querySelector('.recent-name')?.textContent==='Initial'`))
	// A mounted panel ignores unrelated render/volume-style updates.
	run(js(`show({volume:20,commandBusy:true});show({volume:21,commandBusy:false});window.unrelatedReady=false;setTimeout(()=>window.unrelatedReady=true,180)`), wait(`unrelatedReady`))
	var count int
	run(evalInto(`requests.length`, &count))
	if count != 1 {
		t.Fatalf("unrelated renders made %d reads, want1", count)
	}
	// Hold a read through an invalidation burst; its result is superseded.
	run(js(`show({revision:1})`), wait(`requests.length===2`), js(`show({revision:2});show({revision:3})`), js(`window.burstReady=false;setTimeout(()=>window.burstReady=true,180)`), wait(`burstReady`), js(`answer(1,'Stale')`), wait(`requests.length===3`), js(`window.staleReady=false;setTimeout(()=>window.staleReady=true,180)`), wait(`staleReady`))
	var shown string
	run(evalInto(`document.querySelector('.recent-name')?.textContent`, &shown))
	if shown != "Initial" {
		t.Fatalf("superseded response rendered %q, want the previous list (Initial)", shown)
	}
	run(js(`show({revision:4})`), js(`window.followupReady=false;setTimeout(()=>window.followupReady=true,180)`), wait(`followupReady`), js(`requests[2].reject(new Error('temporary'))`), wait(`requests.length===4`), js(`answer(3,'Latest')`), wait(`document.querySelector('.recent-name')?.textContent==='Latest' && !document.querySelector('[role=alert]')`))
	// JSON API failures preserve the valid list, retry recovers, and empty clears.
	run(js(`show({revision:5})`), wait(`requests.length===5`), js(`answer(4,null,false)`), wait(`document.querySelector('[role=alert]')?.textContent.includes('Could not load recents.') && document.querySelector('.recent-name')?.textContent==='Latest'`), chromedp.Click(chromedp.CSS(`[role=alert] button`)), wait(`requests.length===6`), js(`answer(5,'Recovered')`), wait(`document.querySelector('.recent-name')?.textContent==='Recovered' && !document.querySelector('[role=alert]')`))
	run(js(`show({revision:6})`), wait(`requests.length===7`), js(`answer(6,null)`), wait(`document.querySelector('.recents-section')===null`))
	// Browser reconnect and a hidden -> visible transition request fresh history.
	run(js(`show({refreshToken:1})`), wait(`requests.length===8`), js(`answer(7,'Reconnected')`), wait(`document.querySelector('.recent-name')?.textContent==='Reconnected'`))
	// New physical ownership must remove old clickable items immediately.
	var staleVisible bool
	run(evalInto(`show({deviceId:'member',physicalId:'MEMBER'});!!document.querySelector('.recent-item')`, &staleVisible))
	if staleVisible {
		t.Fatal("old target's history remained clickable during switch")
	}
	run(wait(`requests.length===9`), js(`show({epoch:2})`), wait(`requests.length===10`), js(`answer(8,'OldEpoch');answer(9,'MemberHistory')`), wait(`document.querySelector('.recent-name')?.textContent==='MemberHistory'`))
	run(chromedp.Click(chromedp.CSS(`.recent-item`)), wait(`plays.length===1 && plays[0].target==='member' && plays[0].name==='MemberHistory' && writes.length===0`))
	run(js(`foreground(false);foreground(true)`), wait(`requests.length===11`), js(`answer(10,'Foreground')`), wait(`document.querySelector('.recent-name')?.textContent==='Foreground'`))
	run(evalInto(`show({physicalId:'REPLACEMENT'});!!document.querySelector('.recent-item')`, &staleVisible))
	if staleVisible {
		t.Fatal("old physical speaker's history remained clickable")
	}
	run(wait(`requests.length===12`), js(`answer(11,'Replacement')`), wait(`document.querySelector('.recent-name')?.textContent==='Replacement'`))
	run(js(`show({revision:7})`), wait(`requests.length===13`), js(`unmount();answer(12,'Unmounted')`), wait(`document.getElementById('fixture').childElementCount===0 && writes.length===0 && requests[12].options.signal.aborted`))
}

func TestDeviceDetailForwardsRecentsInvalidations(t *testing.T) {
	fixture := strings.TrimSuffix(recentsRefreshFixture, "show();\n") + `
import {DeviceDetail} from '/app/static/js/app.js';
window.detail=(revision=0,token=0)=>render(h(DeviceDetail,{
 deviceId:'speaker',onBack:()=>{},recentsRefreshToken:token,
 devices:{speaker:{info:{name:'Speaker',device_id:'MASTER'},status:{
  epoch:1,revision:revision+1,recentsRevision:revision,
  presets:{Preset:[]},sources:{SourceItem:[]},
  nowPlaying:{Source:'STANDBY'},volume:{ActualVolume:10,MuteEnabled:false}
 }}}
}),document.getElementById('fixture'));
detail();
`
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/*", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
		})
	})
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		navigateFixture(server.URL+"/fixture"),
		chromedp.Poll[chromedp.Void](`requests.length===1`),
		chromedp.Evaluate[chromedp.Void](`answer(0,'First')`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.recent-name')?.textContent==='First'`),
		chromedp.Evaluate[chromedp.Void](`detail(1)`),
		chromedp.Poll[chromedp.Void](`requests.length===2`),
		chromedp.Evaluate[chromedp.Void](`answer(1,'ExternalChange')`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.recent-name')?.textContent==='ExternalChange'`),
		chromedp.Evaluate[chromedp.Void](`detail(1,1)`),
		chromedp.Poll[chromedp.Void](`requests.length===3`),
		chromedp.Evaluate[chromedp.Void](`answer(2,'BrowserReconnect')`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.recent-name')?.textContent==='BrowserReconnect' && writes.length===0`),
	); err != nil {
		t.Fatal(err)
	}
}

func TestAppRetriesFailedRecentsAfterReconnect(t *testing.T) {
	const fixture = `
document.getElementById('fixture').id = 'app';
await import('/app/static/js/app.js');
`
	// Both sockets receive exactly the same device state. A new epoch or
	// revision would exercise a different trigger than the browser reconnect.
	const snapshot = `{"type":"devices","data":{"speaker":{
		"info":{"name":"Speaker","device_id":"MASTER"},
		"status":{"epoch":41,"revision":17,"recentsRevision":9,"connectivity":"online",
			"presets":{"Preset":[]},"sources":{"SourceItem":[]},"nowPlaying":{"Source":"STANDBY"},
			"volume":{"ActualVolume":10,"MuteEnabled":false}}
	}}}`
	var connections, reads atomic.Int32
	upgrader := websocket.Upgrader{}
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/version", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":false}`))
		})
		r.Get("/api/control/ws", func(w http.ResponseWriter, req *http.Request) {
			ws, err := upgrader.Upgrade(w, req, nil)
			if err != nil {
				return
			}
			defer func() { _ = ws.Close() }()
			connections.Add(1)

			if err := ws.WriteMessage(websocket.TextMessage, []byte(snapshot)); err != nil {
				return
			}

			for {
				if _, _, err := ws.ReadMessage(); err != nil {
					return
				}
			}
		})
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			reads.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")

			if connections.Load() < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"success":false}`))

				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"Items":[{"ID":"station","ContentItem":{"Source":"TUNEIN","Location":"/station","ItemName":"Recovered station"}}]}}`))
		})
	})
	proxy := newOutageProxy(t, server.Listener.Addr().String())
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(proxy.url()+"/fixture"),
		chromedp.Click(chromedp.CSS(`button.device-card`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('.recents-section [role=alert]')?.textContent.includes('Could not load recents.')`),
		chromedp.Evaluate[chromedp.Void](`window.openDetail = document.querySelector('.device-detail');
			window.visibilityChanges = 0;
			document.addEventListener('visibilitychange', () => window.visibilityChanges++);`),
	); err != nil {
		t.Fatalf("open device detail and observe the failed Recents request: %v", err)
	}
	if got := reads.Load(); got != 1 {
		t.Fatalf("Recents reads before reconnect = %d, want 1", got)
	}

	proxy.setUp(false)
	if err := chromedp.Do(ctx, chromedp.WaitVisible(chromedp.CSS(`.connection-banner`))); err != nil {
		t.Fatalf("observe socket outage: %v", err)
	}
	proxy.setUp(true)

	if err := chromedp.Do(ctx,
		chromedp.Poll[chromedp.Void](`document.querySelector('.connection-banner') === null`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.recent-name')?.textContent === 'Recovered station'
			&& !document.querySelector('.recents-section [role=alert]')`, chromedp.WithPollingTimeout(5*time.Second)),
	); err != nil {
		t.Fatalf("Recents did not recover after reconnect (%d connections, %d reads): %v", connections.Load(), reads.Load(), err)
	}

	var sameDetail bool
	if err := chromedp.Do(ctx, evalInto(`window.openDetail === document.querySelector('.device-detail')
		&& window.visibilityChanges === 0`, &sameDetail)); err != nil {
		t.Fatal(err)
	}
	if !sameDetail || connections.Load() != 2 || reads.Load() != 2 {
		t.Fatalf("recovery should keep the same visible detail and retry exactly once: same detail %t, %d connections, %d reads",
			sameDetail, connections.Load(), reads.Load())
	}
}
