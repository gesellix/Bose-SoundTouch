//go:build browsertest

package soundtouchweb

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

// awaitPromise makes chromedp.Evaluate wait for an async script and fail on
// its rejection. Without it, Evaluate returns as soon as the script starts,
// and nothing past its first await is checked.
func awaitPromise(p *runtime.EvaluateParams) *runtime.EvaluateParams {
	return p.WithAwaitPromise(true)
}

// TestSettingsOwnershipAndMutationStates exercises the settings component in
// the shipped browser module graph without importing a broader player harness.
func TestSettingsOwnershipAndMutationStates(t *testing.T) {
	app := NewWebApp()
	router := chi.NewRouter()
	app.Mount(router, nil)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(`.nav-discover-icon`, chromedp.ByQuery),
		chromedp.Evaluate(`(async () => {
            const [{ Settings }, { api }, { h, render }] = await Promise.all([
                import('/app/static/js/components/Settings.js'),
                import('/app/static/js/api.js'),
                import('/app/static/lib/preact.module.js'),
            ]);
            const original = {
                settings: api.settings,
                setClockDisplay: api.setClockDisplay,
                setSystemTimeout: api.setSystemTimeout,
                clearBluetoothPairings: api.clearBluetoothPairings,
                confirm: window.confirm,
                fetch: window.fetch,
            };
            const root = document.createElement('div');
            document.body.append(root);
            const loads = [];
            const mutations = [];
            let clearCalls = 0;

            const deferred = () => {
                let resolve;
                let reject;
                const promise = new Promise((res, rej) => {
                    resolve = res;
                    reject = rej;
                });
                return { promise, resolve, reject };
            };
            const snapshot = (label, enabled = false, targetIdentity = 'physical-current') => ({
                targetIdentity,
                support: { clockDisplay: true, sourceNaming: true },
                clockDisplay: { enabled },
                sources: [{ source: 'AUX', displayName: label }],
            });
            const waitFor = async (predicate, message) => {
                for (let attempt = 0; attempt < 100; attempt += 1) {
                    if (predicate()) return;
                    await new Promise(resolve => setTimeout(resolve, 10));
                }
                throw new Error(message);
            };
            const renderTarget = async (deviceId, targetIdentity = 'physical-current') => {
                render(h(Settings, { deviceId, targetIdentity, targetName: deviceId }), root);
                await new Promise(resolve => setTimeout(resolve, 0));
                const details = root.querySelector('details');
                details.open = true;
                details.dispatchEvent(new Event('toggle'));
            };
            const inputValue = () => root.querySelector('.settings-source-row input')?.value;

            try {
                let clearRequest = null;
                window.fetch = async (url, options) => {
                    clearRequest = { url, options };
                    return { json: async () => ({ success: true, data: {} }) };
                };
                await original.clearBluetoothPairings('route-target', 'physical-route');
                window.fetch = original.fetch;
                if (clearRequest?.url !== '/api/control/devices/route-target/settings/bluetooth/pairings?confirmed=true' ||
                    clearRequest?.options?.method !== 'DELETE' ||
                    clearRequest?.options?.headers?.['X-AfterTouch-Settings-Target'] !== 'physical-route') {
                    throw new Error('clear API request is not explicitly confirmed');
                }

                clearRequest = null;
                window.fetch = async (url, options) => {
                    clearRequest = { url, options };
                    return { json: async () => ({ success: true, data: {} }) };
                };
                await original.setSystemTimeout('route-target', 'physical-route', false);
                window.fetch = original.fetch;
                if (clearRequest?.url !== '/api/control/devices/route-target/settings/system-timeout' ||
                    clearRequest?.options?.method !== 'PATCH' ||
                    clearRequest?.options?.headers?.['X-AfterTouch-Settings-Target'] !== 'physical-route' ||
                    clearRequest?.options?.body !== JSON.stringify({ enabled: false })) {
                    throw new Error('automatic standby API request is malformed');
                }

                api.settings = deviceId => {
                    const request = deferred();
                    loads.push({ deviceId, ...request });
                    return request.promise;
                };
                api.setClockDisplay = (deviceId, targetIdentity, body) => {
                    const request = deferred();
                    mutations.push({ deviceId, targetIdentity, body, ...request });
                    return request.promise;
                };

                await renderTarget('stale-load');
                await waitFor(() => loads.length === 1, 'stale load did not start');
                await renderTarget('current-load');
                await waitFor(() => loads.length === 2, 'current load did not start');
                loads[1].resolve({ success: true, data: snapshot('current load') });
                await waitFor(() => inputValue() === 'current load', 'current load did not render');
                loads[0].resolve({ success: true, data: snapshot('stale load data', false, 'physical-stale') });
                await new Promise(resolve => setTimeout(resolve, 20));
                if (inputValue() !== 'current load') {
                    throw new Error('stale settings load replaced the current target');
                }

                root.querySelector('.settings-toggle input').click();
                await waitFor(() => mutations.length === 1, 'stale mutation did not start');
				if (!root.querySelector('.settings-result-busy')) {
					throw new Error('busy mutation state is not visible');
				}
				if (!root.querySelector('.settings-source-row input')?.disabled) {
					throw new Error('busy mutation did not lock controls in other settings sections');
				}
                await renderTarget('current-mutation');
                await waitFor(() => loads.length === 3, 'current mutation target did not load');
                loads[2].resolve({ success: true, data: snapshot('current mutation') });
                await waitFor(() => inputValue() === 'current mutation', 'current mutation target did not render');
                mutations[0].resolve({
                    success: false,
                    outcome: 'unverified',
                    error: 'stale unverified result',
                    data: snapshot('stale mutation data', true),
                });
                await new Promise(resolve => setTimeout(resolve, 20));
                if (inputValue() !== 'current mutation' || root.textContent.includes('stale unverified result')) {
                    throw new Error('stale mutation escaped its target generation');
                }

                root.querySelector('.settings-toggle input').click();
                await waitFor(() => mutations.length === 2, 'verified mutation did not start');
                mutations[1].resolve({
                    success: true,
                    outcome: 'confirmed',
                    warning: 'Verified; full refresh unavailable.',
                });
                await waitFor(() => loads.length === 4, 'confirmed mutation did not refresh stale settings');
                loads[3].reject(new Error('confirmed refresh failed'));
                await waitFor(() => root.querySelector('.settings-load-error')?.textContent.includes(
                    'confirmed refresh failed'), 'failed confirmed refresh is not visible');
                await waitFor(() => root.querySelector('.settings-result-success')?.textContent.includes(
                    'Verified; full refresh unavailable.'), 'confirmed outcome disappeared with its settings section');
                if (root.querySelector('.settings-toggle input')) {
                    throw new Error('confirmed mutation retained controls without current readback');
                }
                root.querySelector('.settings-load-error button').click();
                await waitFor(() => loads.length === 5, 'confirmed settings retry did not start');
                loads[4].resolve({ success: true, data: snapshot('confirmed refresh', true) });
                await waitFor(() => root.querySelector('.settings-toggle input'), 'confirmed settings retry did not render');
                if (!root.querySelector('.settings-toggle input')?.checked) {
                    throw new Error('confirmed mutation retained the stale control value');
                }

                root.querySelector('.settings-toggle input').click();
                await waitFor(() => mutations.length === 3, 'unverified mutation did not start');
                mutations[2].resolve({
                    success: false,
                    outcome: 'unverified',
                    error: 'readback unavailable',
                    data: snapshot('unverified mutation', true),
                });
                await waitFor(() => root.querySelector('.settings-result-unverified')?.textContent.includes('readback unavailable'),
                    'unverified result is not visible');

                root.querySelector('.settings-toggle input').click();
                await waitFor(() => mutations.length === 4, 'failed mutation did not start');
                mutations[3].reject(new Error('mutation failed'));
                await waitFor(() => root.querySelector('.settings-result-error')?.textContent.includes('mutation failed'),
                    'error result is not visible');

                api.clearBluetoothPairings = async (deviceId, targetIdentity) => {
                    clearCalls += 1;
                    return {
                        success: false,
                        outcome: 'unverified',
                        error: 'clear unverified for ' + deviceId,
                        data: {
                            targetIdentity,
                            support: { bluetooth: true, bluetoothClear: true },
                            bluetooth: { connectionStatus: 'READY' },
                            // As the API sends it: the unverified explanation
                            // travels in the snapshot's errors too.
                            errors: { bluetoothClear: 'clear unverified for ' + deviceId },
                        },
                    };
                };
                await renderTarget('clear-target');
                await waitFor(() => loads.length === 6, 'clear target did not load');
                loads[5].resolve({
                    success: true,
                    data: {
                        targetIdentity: 'physical-current',
                        support: { bluetooth: true, bluetoothClear: true },
                        bluetooth: { connectionStatus: 'READY' },
                    },
                });
                await waitFor(() => Array.from(root.querySelectorAll('button')).some(
                    button => button.textContent.trim() === 'Clear all pairings'), 'clear action did not render');
                const clearButton = Array.from(root.querySelectorAll('button')).find(
                    button => button.textContent.trim() === 'Clear all pairings');
                window.confirm = () => false;
                clearButton.click();
                await new Promise(resolve => setTimeout(resolve, 20));
                if (clearCalls !== 0) throw new Error('cancelled clear sent a request');
                window.confirm = () => true;
                clearButton.click();
                await waitFor(() => clearCalls === 1, 'confirmed clear did not send exactly one request');
                await waitFor(() => root.querySelector('.settings-result-unverified'),
                    'clear unverified result is not visible');
                await new Promise(resolve => setTimeout(resolve, 50));
                if (root.querySelector('.settings-error')) {
                    throw new Error('unverified clear was also shown as a section error: ' +
                        root.querySelector('.settings-error').textContent);
                }

                await renderTarget('identity-mismatch', 'physical-expected');
                await waitFor(() => loads.length === 7, 'identity-mismatch target did not load');
                loads[6].resolve({
                    success: true,
                    data: snapshot('replacement speaker', false, 'physical-replacement'),
                });
                await waitFor(() => root.querySelector('.settings-load-error')?.textContent.includes(
                    'physical speaker changed'), 'identity mismatch was not rejected');
                if (root.querySelector('.settings-toggle')) {
                    throw new Error('identity mismatch exposed mutable settings controls');
                }

                const mutationTargets = mutations.map(request => request.deviceId);
                if (mutationTargets.join(',') !== 'current-load,current-mutation,current-mutation,current-mutation') {
                    throw new Error('mutations reached unexpected targets: ' + mutationTargets.join(','));
                }
                if (mutations.some(request => request.targetIdentity !== 'physical-current')) {
                    throw new Error('mutation escaped its physical target generation');
                }
            } finally {
                render(null, root);
                root.remove();
                api.settings = original.settings;
                api.setClockDisplay = original.setClockDisplay;
                api.setSystemTimeout = original.setSystemTimeout;
                api.clearBluetoothPairings = original.clearBluetoothPairings;
                window.confirm = original.confirm;
                window.fetch = original.fetch;
            }
        })()`, nil, awaitPromise),
	); err != nil {
		t.Fatalf("settings browser contract: %v", err)
	}
}

// TestSettingsStandbyAndWiFiOnboardingBrowserContract keeps automatic standby
// readback-driven while exposing Wi-Fi setup only as a separately launched
// workflow.
func TestSettingsStandbyAndWiFiOnboardingBrowserContract(t *testing.T) {
	app := NewWebApp()
	router := chi.NewRouter()
	app.Mount(router, nil)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(`.nav-discover-icon`, chromedp.ByQuery),
		chromedp.Evaluate(`(async () => {
            const [{ Settings }, { api }, { h, render }] = await Promise.all([
                import('/app/static/js/components/Settings.js'),
                import('/app/static/js/api.js'),
                import('/app/static/lib/preact.module.js'),
            ]);
            const original = {
                settings: api.settings,
                setSystemTimeout: api.setSystemTimeout,
            };
            const root = document.createElement('div');
            document.body.append(root);
            const calls = [];
            const snapshot = enabled => ({
                targetIdentity: 'standby-physical',
                support: { systemTimeout: true, wifiOnboarding: true },
                systemTimeout: { enabled },
                onboardingUrl: '/setup/',
            });
            const waitFor = async (predicate, message) => {
                for (let attempt = 0; attempt < 100; attempt += 1) {
                    if (predicate()) return;
                    await new Promise(resolve => setTimeout(resolve, 10));
                }
                throw new Error(message);
            };

            try {
                api.settings = async () => ({ success: true, data: snapshot(true) });
                api.setSystemTimeout = async (deviceId, targetIdentity, enabled) => {
                    calls.push({ deviceId, targetIdentity, enabled });
                    return { success: true, data: snapshot(enabled) };
                };

                render(h(Settings, {
                    deviceId: 'standby-target',
                    targetIdentity: 'standby-physical',
                    targetName: 'Test speaker',
                }), root);
                await new Promise(resolve => setTimeout(resolve, 0));
                const details = root.querySelector('details');
                details.open = true;
                details.dispatchEvent(new Event('toggle'));

                await waitFor(() => root.querySelector('.settings-toggle input'),
                    'automatic standby toggle did not render');
                const link = root.querySelector('a.settings-command-link');
                if (!link || link.getAttribute('href') !== '/setup/' ||
                    link.getAttribute('target') !== '_blank' || link.getAttribute('rel') !== 'noopener') {
                    throw new Error('Wi-Fi onboarding was not rendered as a separated safe link');
                }

                root.querySelector('.settings-toggle input').click();
                await waitFor(() => calls.length === 1, 'automatic standby mutation did not run');
                if (calls[0].deviceId !== 'standby-target' ||
                    calls[0].targetIdentity !== 'standby-physical' || calls[0].enabled !== false) {
                    throw new Error('automatic standby mutation used the wrong target or value');
                }
                await waitFor(() => root.querySelector('.settings-result-success'),
                    'verified automatic standby result did not render');
            } finally {
                render(null, root);
                root.remove();
                api.settings = original.settings;
                api.setSystemTimeout = original.setSystemTimeout;
            }
        })()`, nil, awaitPromise),
	); err != nil {
		t.Fatalf("settings standby/onboarding browser contract: %v", err)
	}
}

// TestSettingsNarrowTouchAndKeyboardContract exercises the responsive layout
// and real browser input paths without broadening the player test harness.
func TestSettingsNarrowTouchAndKeyboardContract(t *testing.T) {
	app := NewWebApp()
	router := chi.NewRouter()
	app.Mount(router, nil)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	ctx := newHeadlessChromeContext(t)
	const toggle = `#settings-input-contract .settings-toggle input`
	const toggleLabel = `#settings-input-contract .settings-toggle`
	const summary = `#settings-input-contract .settings-summary`
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(390, 844, chromedp.EmulateMobile, chromedp.EmulateTouch),
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(`.nav-discover-icon`, chromedp.ByQuery),
		chromedp.Evaluate(`(async () => {
            const [{ Settings }, { api }, { h, render }] = await Promise.all([
                import('/app/static/js/components/Settings.js'),
                import('/app/static/js/api.js'),
                import('/app/static/lib/preact.module.js'),
            ]);
            const original = {
                settings: api.settings,
                setSystemTimeout: api.setSystemTimeout,
            };
            const root = document.createElement('div');
            root.id = 'settings-input-contract';
            root.className = 'device-detail';
            document.body.append(root);
            const calls = [];
            const snapshot = enabled => ({
                targetIdentity: 'narrow-physical',
                support: {
                    systemTimeout: true,
                    language: true,
                    sync: true,
                    sourceNaming: true,
                    wifiOnboarding: true,
                },
                systemTimeout: { enabled },
                language: {
                    code: 0,
                    options: [
                        { code: 0, name: 'English' },
                        { code: 1, name: 'Long localized language option' },
                    ],
                },
                sync: { mode: 'SYNC_TO_ZONE' },
                sources: [{
                    source: 'AUX',
                    sourceAccount: 'AUXILIARY-SOURCE-WITH-A-LONG-IDENTIFIER',
                    displayName: 'A long source name that must stay inside the narrow settings panel',
                }],
                onboardingUrl: '/setup/',
            });
            api.settings = async () => ({ success: true, data: snapshot(true) });
            api.setSystemTimeout = async (deviceId, targetIdentity, enabled) => {
                calls.push({ deviceId, targetIdentity, enabled });
                return { success: true, outcome: 'confirmed', data: snapshot(enabled) };
            };
            render(h(Settings, {
                deviceId: 'narrow-target',
                targetIdentity: 'narrow-physical',
                targetName: 'A speaker with a long localized display name',
            }), root);
            await new Promise(resolve => setTimeout(resolve, 0));
            const details = root.querySelector('details');
            details.open = true;
            details.dispatchEvent(new Event('toggle'));
            window.__settingsInputContract = { calls, original, root, api, render };
		})()`, nil, awaitPromise),
		chromedp.WaitVisible(toggle, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("settings narrow setup: %v", err)
	}

	var overflow []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
        const root = document.getElementById('settings-input-contract');
        const nodes = [root, ...root.querySelectorAll(
            '.settings-content, .settings-group, .settings-field, .settings-segmented, .settings-source-row')];
        return nodes.filter(node => node.scrollWidth > node.clientWidth + 1)
            .map(node => node.className || node.tagName);
    })()`, &overflow)); err != nil {
		t.Fatalf("settings narrow layout inspection: %v", err)
	}
	if len(overflow) != 0 {
		t.Fatalf("settings narrow layout overflow: %v", overflow)
	}

	var focusVisible bool
	if err := chromedp.Run(ctx,
		chromedp.Focus(summary, chromedp.ByQuery),
		chromedp.KeyEvent("\t"),
		chromedp.Poll(`document.activeElement ===
            document.querySelector('#settings-input-contract .settings-toggle input')`, nil),
		chromedp.Evaluate(`(() => {
            const target = document.querySelector('#settings-input-contract .settings-toggle input');
            const track = target?.nextElementSibling;
            const style = track ? getComputedStyle(track) : null;
            return document.activeElement === target && style && style.outlineStyle !== 'none' &&
                parseFloat(style.outlineWidth) > 0;
        })()`, &focusVisible),
	); err != nil {
		t.Fatalf("settings keyboard focus: %v", err)
	}
	if !focusVisible {
		t.Fatal("settings keyboard focus ring is not visible")
	}

	if err := chromedp.Run(ctx,
		chromedp.KeyEvent(" "),
		chromedp.Poll(`window.__settingsInputContract.calls.length === 1 &&
            window.__settingsInputContract.calls[0].enabled === false &&
            document.querySelector('#settings-input-contract .settings-toggle input')?.checked === false`, nil),
		chromedp.ScrollIntoView(toggleLabel, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("settings keyboard activation: %v", err)
	}

	var point [2]float64
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
        const rect = document.querySelector('#settings-input-contract .settings-toggle').getBoundingClientRect();
        return [rect.left + rect.width / 2, rect.top + rect.height / 2];
    })()`, &point)); err != nil {
		t.Fatalf("settings touch target: %v", err)
	}

	touch := []*input.TouchPoint{{X: point[0], Y: point[1], ID: 1}}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchTouchEvent(input.TouchStart, touch).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchTouchEvent(input.TouchEnd, nil).Do(ctx)
		}),
		chromedp.Poll(`window.__settingsInputContract.calls.length === 2 &&
            window.__settingsInputContract.calls[1].enabled === true &&
            document.querySelector('#settings-input-contract .settings-toggle input')?.checked === true`, nil),
		chromedp.Evaluate(`(() => {
            const fixture = window.__settingsInputContract;
            fixture.render(null, fixture.root);
            fixture.root.remove();
            fixture.api.settings = fixture.original.settings;
            fixture.api.setSystemTimeout = fixture.original.setSystemTimeout;
            delete window.__settingsInputContract;
        })()`, nil),
	); err != nil {
		t.Fatalf("settings narrow touch contract: %v", err)
	}
}
