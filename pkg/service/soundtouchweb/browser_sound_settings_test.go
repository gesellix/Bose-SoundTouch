//go:build browsertest

package soundtouchweb

import (
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

func TestScopedSoundSettingsDiscreteWritesAndHierarchy(t *testing.T) {
	app := NewWebApp()
	router := chi.NewRouter()
	app.Mount(router, nil)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(chromedp.CSS(`.nav-discover-icon`)),
		chromedp.Evaluate[chromedp.Void](`(async () => {
            const [{ MemberSettings }, { api }, { h, render }] = await Promise.all([
                import('/app/static/js/components/MemberSettings.js'),
                import('/app/static/js/api.js'),
                import('/app/static/lib/preact.module.js'),
            ]);
            const original = { settings: api.settings, bass: api.bass, balance: api.balance, device: api.device, bassChecked: api.bassChecked, balanceChecked: api.balanceChecked };
            api.bassChecked = (...args) => api.bass(...args); api.balanceChecked = (...args) => api.balance(...args);
            const root = document.createElement('div');
            document.body.append(root);
            const bass = { 'left.local': 0, 'right.local': 0 };
            const writes = [];
            let balance = 0;
            const physical = (id, role) => ({ controlId: id + '.local', physicalId: id,
                name: id + ' speaker', role,
                status: { epoch: 1, revision: 1, connectivity: 'online', bass: { ActualBass: 0 } } });
            const left = physical('left', 'LEFT');
            const right = physical('right', 'RIGHT');
            const pair = { info: { device_id: 'right', name: 'Room pair' },
                settingsTargets: [right, left],
                stereoPair: { id: 'pair', name: 'Room pair', masterDeviceId: 'right',
                    members: [{ deviceId: 'right', role: 'RIGHT' }, { deviceId: 'left', role: 'LEFT' }] },
                status: { epoch: 1, revision: 1, connectivity: 'online',
                    group: { ID: 'pair', MasterDeviceID: 'right', Roles: { Roles: [
                        { DeviceID: 'right', Role: 'RIGHT' }, { DeviceID: 'left', Role: 'LEFT' }] } },
                    balance: { Available: true, Min: -7, Max: 7, Default: 0, Actual: 0 } } };
            left.status.group = pair.status.group; right.status.group = pair.status.group;
            const check = (condition, message) => { if (!condition) throw new Error(message); };
            const waitFor = async (predicate, message) => {
                for (let attempt = 0; attempt < 100; attempt += 1) {
                    if (predicate()) return;
                    await new Promise(resolve => setTimeout(resolve, 10));
                }
                throw new Error(message);
            };
            api.settings = async id => ({ success: true, data: { targetIdentity: id.split('.')[0], support: {},
                bassCapabilities: { BassAvailable: true, BassMin: -9, BassMax: 0, BassDefault: 0 },
                statusEpoch: 1, statusRevision: 1, bass: { ActualBass: bass[id] } } });
            api.bass = async (id, value) => { writes.push(['bass', id, value]); bass['left.local'] = value; bass['right.local'] = value; return { success: true }; };
            api.balance = async (id, value) => { writes.push(['balance', id, value]); balance = value; return { success: true }; };
            api.device = async () => ({ success: true, data: { ...pair,
                status: { ...pair.status, balance: { ...pair.status.balance, Actual: balance } } } });
            try {
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                const disclosure = root.querySelector('.member-settings');
                check(disclosure && !disclosure.open, 'Member Settings must start collapsed');
                disclosure.open = true;
                await waitFor(() => root.querySelectorAll('.sound-control').length === 2, 'Reported scoped controls did not load');
                const summaries = [...root.querySelectorAll('.settings-section > summary')].map(node => node.textContent.trim());
                check(summaries.length === 3 && summaries[0].includes('Settings') && summaries[1].includes('LEFT') && summaries[2].includes('RIGHT'), 'One Settings must contain deterministic device groups');
                check(!root.querySelector('.sound-settings > summary'), 'Nested Sound must be subordinate');
                check([...root.querySelectorAll('.settings-section')].slice(1).every(node => !node.open), 'Physical device groups must start collapsed');
                const controls = [...root.querySelectorAll('.sound-control')];
                const settled = (node, value) => node.querySelector('output').textContent === value
                    && node.getAttribute('aria-busy') === 'false' && !node.querySelector('.sound-control-readback');
                check(controls[0].textContent.includes('Room pair') && controls[1].textContent.includes('Room pair'), 'Bass and balance must each identify the logical pair');
                const groups = [...root.querySelectorAll('.member-settings-content > .settings-section')];
                const rotation = node => getComputedStyle(node.querySelector(':scope > summary > .settings-chevron')).transform;
                await new Promise(resolve => setTimeout(resolve, 180));
                check(rotation(groups[0]) !== rotation(disclosure), 'Open Settings rotated the collapsed child chevron');
                groups[0].open = true;
                await new Promise(resolve => setTimeout(resolve, 180));
                check(rotation(groups[0]) === rotation(disclosure) && rotation(groups[1]) !== rotation(disclosure), 'Each chevron must follow only its own disclosure');
                groups[0].open = false;
                await new Promise(resolve => setTimeout(resolve, 180));
                check(rotation(groups[0]) !== rotation(disclosure), 'Collapsed child chevron stayed open');
                for (const button of root.querySelectorAll('button')) { button.focus(); button.blur(); }
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await new Promise(resolve => setTimeout(resolve, 20));
                check(writes.length === 0, 'Focus, blur or rerender synthesized a sound write');
                controls[0].querySelector('button').click();
                await waitFor(() => settled(controls[0], '-1'), 'Bass write was not confirmed');
                check(writes.length === 1 && writes[0].join('|') === 'bass|right.local|-1', 'Pair bass must write once to the confirmed RIGHT master');
                controls[0].querySelector('[data-action="reset"]').click();
                await waitFor(() => writes.length === 2 && settled(controls[0], '0'), 'Bass reset was not confirmed');
                controls[1].querySelector('[data-action="increase"]').click();
                await waitFor(() => writes.length === 3 && settled(controls[1], 'Right 1'), 'Pair balance was not confirmed');
                check(writes[2].join('|') === 'balance|right.local|1', 'Balance must follow the confirmed RIGHT master');
                api.bass = async () => ({ success: false, error: 'Final bass write failed' });
                controls[0].querySelector('button').click();
                await waitFor(() => controls[0].textContent.includes('Final bass write failed'), 'Final failure disappeared');
                check(controls[0].textContent.includes('Speaker: 0'), 'Failure must retain the actual speaker readback');
                for (const button of controls[0].querySelectorAll('button')) { button.focus(); button.blur(); }
                check(controls[0].textContent.includes('Final bass write failed'), 'Blur cleared a final failure');
                pair.settingsTargets = [right, { ...left, status: { ...left.status, revision: 2, bass: { ActualBass: -4 } } }];
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await waitFor(() => root.textContent.includes('different bass values'), 'Pair disagreement was not explained');
                check(!controls[0].querySelector('button').disabled, 'Transient pair disagreement discarded rapid choices');
                check(controls[0].textContent.includes('Final bass write failed'), 'One physical bass update cleared pair failure');
                right.status = { ...right.status, revision: 2, bass: { ActualBass: -4 } };
                bass['left.local'] = -4; bass['right.local'] = -4;
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await waitFor(() => settled(controls[0], '-4') && !controls[0].textContent.includes('Final bass write failed'), 'External child readback did not converge');
                let settleBass;
                let bassStarted = false;
                api.bass = () => {
                    bassStarted = true;
                    return new Promise(resolve => { settleBass = resolve; });
                };
                controls[0].querySelector('button').click();
                await waitFor(() => bassStarted, 'Bass interaction was not dispatched');
                pair.settingsTargets = [right, { ...left, status: { ...left.status, revision: 3,
                    connectivity: 'offline', bass: null, volume: { ActualVolume: 30 } } }];
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await waitFor(() => controls[0].querySelector('button').disabled, 'Unavailable bass did not disable writes');
                check(controls[0].isConnected && root.querySelectorAll('.sound-control').length === 2,
                    'Same physical target lost its mounted sound control');
                settleBass({ success: false, error: 'Delayed unavailable bass failure' });
                await waitFor(() => controls[0].textContent.includes('Delayed unavailable bass failure'), 'Availability loss discarded pending bass outcome');
                pair.settingsTargets = [right, { ...left, status: { ...left.status, revision: 4,
                    connectivity: 'offline', bass: null, volume: { ActualVolume: 40 } } }];
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await new Promise(resolve => setTimeout(resolve, 20));
                check(controls[0].textContent.includes('Delayed unavailable bass failure'), 'Offline volume update cleared bass error');
                pair.settingsTargets = [right, { ...left, status: { ...left.status, revision: 5, bass: { ActualBass: -4 } } }];
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await waitFor(() => !controls[0].querySelector('button').disabled, 'Recovered bass stayed disabled');
                check(controls[0].textContent.includes('Delayed unavailable bass failure'), 'Availability recovery alone cleared final bass error');

                let settleBalance;
                let balanceStarted = false;
                api.balance = () => {
                    balanceStarted = true;
                    return new Promise(resolve => { settleBalance = resolve; });
                };
                controls[1].querySelector('button').click();
                await waitFor(() => balanceStarted, 'Balance interaction was not dispatched');
                pair.status = { ...pair.status, revision: 2, connectivity: 'offline', group: null,
                    balance: { ...pair.status.balance, Available: false }, volume: { ActualVolume: 30 } };
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await waitFor(() => controls[1].querySelector('button').disabled, 'Unavailable balance did not disable writes');
                check(controls[1].isConnected, 'Availability loss unmounted pair balance');
                settleBalance({ success: false, error: 'Delayed unavailable balance failure' });
                await waitFor(() => controls[1].textContent.includes('Delayed unavailable balance failure'), 'Unavailable pair discarded pending final error');
                pair.status = { ...pair.status, revision: 3, volume: { ActualVolume: 40 } };
                render(h(MemberSettings, { deviceId: 'right.local', device: pair }), root);
                await new Promise(resolve => setTimeout(resolve, 20));
                check(controls[1].textContent.includes('Delayed unavailable balance failure'), 'Unrelated master volume projection cleared balance error');
                window.__scopedSoundSettingsResult = { ok: true };
            } catch (error) {
                window.__scopedSoundSettingsResult = { ok: false, error: error.stack || String(error) };
            } finally {
                render(null, root);
                root.remove();
                Object.assign(api, original);
            }
        })()`, chromedp.EvalAwaitPromise),
		chromedp.WaitVisible(chromedp.CSS(`.nav-discover-icon`)),
		chromedp.Poll[chromedp.Void](`window.__scopedSoundSettingsResult !== undefined`),
	); err != nil {
		t.Fatal(err)
	}
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := chromedp.Do(ctx, evalInto(`window.__scopedSoundSettingsResult`, &result)); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatal(result.Error)
	}
}

func TestScopedSoundSettingsConnectionEpochOwnership(t *testing.T) {
	app := NewWebApp()
	router := chi.NewRouter()
	app.Mount(router, nil)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(chromedp.CSS(`.nav-discover-icon`)),
		chromedp.Evaluate[chromedp.Void](`(async () => {
            const [{ SteppedSoundControl }, { SoundSettings }, { api }, { h, render }] = await Promise.all([
                import('/app/static/js/components/SteppedSoundControl.js'),
                import('/app/static/js/components/SoundSettings.js'),
                import('/app/static/js/api.js'),
                import('/app/static/lib/preact.module.js'),
            ]);
            const original = { settings: api.settings, bass: api.bass, bassChecked: api.bassChecked };
            api.bassChecked = (...args) => api.bass(...args);
            const root = document.createElement('div');
            document.body.append(root);
            const check = (condition, message) => { if (!condition) throw new Error(message); };
            const waitFor = async (predicate, message) => {
                for (let attempt = 0; attempt < 100; attempt += 1) {
                    if (predicate()) return;
                    await new Promise(resolve => setTimeout(resolve, 10));
                }
                throw new Error(message);
            };
            const target = epoch => ({ controlId: 'speaker.local', physicalId: 'speaker', name: 'Speaker',
                status: { epoch, revision: 1, connectivity: 'online', bass: { ActualBass: 0 } } });
            const control = { min: -9, max: 0, defaultValue: 0, value: 0 };
            const snapshot = (epoch, max = 0, actual = 0) => ({ success: true, data: {
                targetIdentity: 'speaker', statusEpoch: epoch, statusRevision: 1,
                bassCapabilities: { BassAvailable: true, BassMin: -9, BassMax: max, BassDefault: 0 },
                bass: { ActualBass: actual },
            } });
            try {
                let settleWrite;
                const writes = [];
                api.bass = (id, value) => {
                    writes.push([id, value]);
                    return new Promise(resolve => { settleWrite = resolve; });
                };
                api.settings = async () => snapshot(1, 0, -1);
                const renderControl = epoch => render(h(SteppedSoundControl, {
                    kind: 'bass', target: target(epoch), control, ownerKey: 'same-physical-target',
                }), root);
                renderControl(1);
                root.querySelector('button').click();
                await waitFor(() => writes.length === 1 && root.querySelector('output').textContent === '-1',
                    'Old epoch interaction was not dispatched');
                root.querySelector('button').click();
                await waitFor(() => root.querySelector('output').textContent === '-2', 'Second old epoch click was not queued');
                renderControl(2);
                await waitFor(() => root.querySelector('output').textContent === '0', 'New connection did not reset old preview');
                settleWrite({ success: true });
                await waitFor(() => root.querySelector('.sound-control').getAttribute('aria-busy') === 'false',
                    'Old connection request did not settle');
                check(writes.length === 1, 'Queued old epoch value was sent to the new connection');
                check(root.querySelector('output').textContent === '0' && !root.querySelector('.sound-control-readback'),
                    'Old successful readback overwrote new connection state');

                render(null, root);
                let settleOldSettings;
                const loads = [];
                api.settings = () => {
                    loads.push(true);
                    return loads.length === 1 ? new Promise(resolve => { settleOldSettings = resolve; })
                        : Promise.resolve(snapshot(2));
                };
                const renderSound = epoch => render(h(SoundSettings, {
                    deviceId: 'speaker.local', nested: true, active: true,
                    device: { info: { device_id: 'speaker', name: 'Speaker' }, settingsTargets: [target(epoch)] },
                }), root);
                renderSound(1);
                await waitFor(() => loads.length === 1, 'Initial settings read did not start');
                renderSound(2);
                await waitFor(() => loads.length === 2 && root.querySelector('.sound-control'),
                    'New connection did not reload reported acoustic capabilities');
                settleOldSettings(snapshot(1, 9, -1));
                await new Promise(resolve => setTimeout(resolve, 30));
                const scale = root.querySelector('.sound-control-scale');
                check(scale.getAttribute('aria-valuemin') === '-9' && scale.getAttribute('aria-valuemax') === '0',
                    'Old connection settings capabilities replaced new readback');
                check(scale.getAttribute('aria-valuenow') === '0' && root.querySelector('output').textContent === '0',
                    'Old settings value settled into new connection');
            } finally {
                render(null, root);
                root.remove();
                Object.assign(api, original);
            }
        })()`, chromedp.EvalAwaitPromise),
	); err != nil {
		t.Fatalf("connection epoch ownership browser contract: %v", err)
	}
}

func TestScopedSoundSettingsTapAndRapidSteps(t *testing.T) {
	app := NewWebApp()
	router := chi.NewRouter()
	app.Mount(router, nil)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(chromedp.CSS(`.nav-discover-icon`)),
		chromedp.Evaluate[chromedp.Void](`(async () => {
            const [{ SteppedSoundControl }, { api }, { h, render }] = await Promise.all([
                import('/app/static/js/components/SteppedSoundControl.js'),
                import('/app/static/js/api.js'), import('/app/static/lib/preact.module.js'),
            ]);
            const original = { settings: api.settings, bass: api.bass, bassChecked: api.bassChecked };
            api.bassChecked = (...args) => api.bass(...args);
            const root = document.createElement('div'); root.style.maxWidth = '320px'; document.body.append(root);
            const check = (ok, why) => { if (!ok) throw new Error(why); };
            const wait = async (f, why) => {
                for (let n = 0; n < 150; n++) { if (f()) return; await new Promise(r => setTimeout(r, 10)); }
                throw new Error(why);
            };
            const target = { controlId: 'speaker.local', physicalId: 'speaker', name: 'Speaker',
                status: { epoch: 1, revision: 1, connectivity: 'online', bass: { ActualBass: 0 } } };
            const control = { min: -9, max: 0, value: 0, defaultValue: 0 };
            const caps = { BassAvailable: true, BassMin: -9, BassMax: 0, BassDefault: 0 };
            let actual = 0, releaseFirst, active = 0, maxActive = 0;
            const writes = [];
            api.bass = async (id, value) => {
                writes.push(value); active++; maxActive = Math.max(maxActive, active);
                if (writes.length === 1) await new Promise(resolve => { releaseFirst = resolve; });
                actual = value; active--; return { success: true };
            };
            api.settings = async () => ({ success: true, data: { targetIdentity: 'speaker', statusEpoch: 1,
                statusRevision: writes.length + 1, bassCapabilities: caps, bass: { ActualBass: actual } } });
            try {
                render(h(SteppedSoundControl, { kind: 'bass', target, control, ownerKey: 'owner' }), root);
                const decrease = root.querySelector('[data-action="decrease"]');
                decrease.click(); decrease.click(); decrease.click();
                check(writes.length === 1 && writes[0] === -1, 'First step must send immediately, with one active write');
                await wait(() => root.querySelector('output').textContent === '-3', 'Synchronous clicks lost their accumulated steps');
                check(root.textContent.includes('Speaker: 0'), 'Pending preview must show the actual speaker readback');
                releaseFirst();
                await wait(() => writes.length === 2 && root.querySelector('.sound-control').getAttribute('aria-busy') === 'false', 'Latest waiting step did not settle');
                check(writes.join(',') === '-1,-3' && maxActive === 1 && actual === -3, 'Rapid steps must preserve latest absolute value and serialize writes');
                check(!root.querySelector('.sound-control-readback'), 'Confirmed value must not repeat speaker readback');
                const scale = root.querySelector('.sound-control-scale');
                check(scale && scale.getAttribute('role') === 'slider' && scale.tabIndex === 0, 'Scale must be keyboard accessible');
                // Synthetic PointerEvents do not enter Chromium's native active-pointer registry.
                scale.setPointerCapture = () => {};
                const rect = scale.getBoundingClientRect();
                const pointer = (type, ratio, id=1) => scale.dispatchEvent(new PointerEvent(type, {
                    bubbles: true, pointerId: id, pointerType: 'mouse', button: 0, isPrimary: true,
                    clientX: rect.left + rect.width * ratio, clientY: rect.top + rect.height / 2,
                }));
                pointer('pointerdown', .3); pointer('pointerup', .3);
                await wait(() => writes.length === 3 && actual === -6, 'Tap must choose the nearest reported integer');
                await wait(() => root.querySelector('.sound-control').getAttribute('aria-busy') === 'false', 'Tap did not settle');
                scale.dispatchEvent(new MouseEvent('click', { bubbles: true }));
                pointer('pointerdown', .1); pointer('pointermove', .8); pointer('pointerup', .8);
                pointer('pointerdown', .2); pointer('pointercancel', .2); pointer('pointerup', .2);
                scale.focus(); scale.blur();
                await new Promise(r => setTimeout(r, 30));
                check(writes.length === 3, 'Drag, cancellation, follow-up click or blur sent a write');
                scale.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'ArrowRight' }));
                await wait(() => actual === -5 && writes.length === 4, 'Keyboard step did not use the current requested value');
                await wait(() => root.querySelector('.sound-control').getAttribute('aria-busy') === 'false', 'Keyboard step did not settle');
                root.querySelector('[data-action="reset"]').click();
                await wait(() => actual === 0 && writes.length === 5, 'Reset did not select the reported default once');
                await wait(() => root.querySelector('.sound-control').getAttribute('aria-busy') === 'false', 'Reset did not settle');
                render(h(SteppedSoundControl, { kind: 'bass', target, control, ownerKey: 'owner', available: false }), root);
                pointer('pointerdown', .2); pointer('pointerup', .2);
                scale.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Home' }));
                await new Promise(r => setTimeout(r, 30));
                check(writes.length === 5, 'Disabled scale sent a write');
            } finally { render(null, root); root.remove(); Object.assign(api, original); }
        })()`, chromedp.EvalAwaitPromise),
	); err != nil {
		t.Fatalf("tap and rapid sound steps: %v", err)
	}
}

func TestScopedSoundSettingsCompactGeometry(t *testing.T) {
	for _, viewport := range []struct {
		name          string
		width, height int64
	}{
		{"desktop", 1280, 800},
		{"iPad", 768, 1024},
	} {
		t.Run(viewport.name, func(t *testing.T) {
			app := NewWebApp()
			router := chi.NewRouter()
			app.Mount(router, nil)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			ctx := newHeadlessChromeContext(t)
			if err := chromedp.Do(ctx,
				chromedp.EmulateViewport(viewport.width, viewport.height),
				chromedp.Navigate(server.URL+"/app"),
				chromedp.WaitVisible(chromedp.CSS(`.nav-discover-icon`)),
				chromedp.Evaluate[chromedp.Void](`(async () => {
                    const [{ SteppedSoundControl }, { h, render }] = await Promise.all([
                        import('/app/static/js/components/SteppedSoundControl.js'),
                        import('/app/static/lib/preact.module.js'),
                    ]);
                    const root = document.createElement('div');
                    root.style.maxWidth = '320px'; document.body.append(root);
                    const check = (ok, why) => { if (!ok) throw new Error(why); };
                    const target = { controlId: 'speaker.local', physicalId: 'speaker', name: 'Room speaker',
                        status: { epoch: 1, revision: 1, connectivity: 'online' } };
                    try {
                        for (const kind of ['bass', 'balance']) {
                            const control = { min: kind === 'bass' ? -9 : -7, max: kind === 'bass' ? 0 : 7,
                                value: -3, defaultValue: 0 };
                            render(h(SteppedSoundControl, { kind, target, control, ownerKey: kind }), root);
                            const sound = root.querySelector('.sound-control');
                            const scale = sound.querySelector('.sound-control-scale');
                            const decrease = sound.querySelector('[data-action="decrease"]').getBoundingClientRect();
                            const increase = sound.querySelector('[data-action="increase"]').getBoundingClientRect();
                            const area = scale.getBoundingClientRect();
                            check(decrease.width >= 44 && decrease.height >= 44 && increase.width >= 44 && increase.height >= 44,
                                kind + ': side buttons lost their touch area');
                            check(area.width >= 44 && area.height >= 44 && Math.abs(decrease.top - area.top) < 1
                                && Math.abs(increase.top - area.top) < 1 && decrease.right < area.left && area.right < increase.left,
                                kind + ': scale must remain between side buttons in one 44px row');
                            check(sound.getBoundingClientRect().height <= 180, kind + ': settled control is no longer compact');
                            const output = sound.querySelector('output');
                            check(output.getAttribute('aria-live') === 'polite'
                                && output.getBoundingClientRect().top < area.top, 'Current value must remain in the heading');
                            check(sound.querySelector('.sound-control-scope').textContent === target.name, 'Scope label disappeared');
                            check(scale.getAttribute('role') === 'slider' && scale.getAttribute('aria-valuemin') === String(control.min)
                                && scale.getAttribute('aria-valuemax') === String(control.max), 'Compact scale lost its accessible range');
                            const selected = sound.querySelector('.sound-control-tick.selected');
                            const ordinary = sound.querySelector('.sound-control-tick:not(.selected):not(.default)');
                            check(selected.getBoundingClientRect().height > ordinary.getBoundingClientRect().height
                                && ordinary.getBoundingClientRect().width > ordinary.getBoundingClientRect().height,
                                'Selected step must stand above short horizontal segments');
                            check(sound.querySelector('.sound-control-footer').textContent.includes(kind === 'balance' ? 'Default Centered' : 'Default 0'),
                                'Footer must format the reported default');
                            check(!sound.querySelector('.sound-control-readback') && !sound.textContent.includes('Range')
                                && !sound.textContent.includes('Tap the scale'), 'Settled control repeats routine guidance or readback');
                        }
                    } finally { render(null, root); root.remove(); }
                })()`, chromedp.EvalAwaitPromise),
			); err != nil {
				t.Fatalf("compact sound geometry: %v", err)
			}
		})
	}
}
