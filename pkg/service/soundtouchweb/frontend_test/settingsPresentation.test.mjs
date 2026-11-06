import assert from 'node:assert/strict';
import test from 'node:test';

import {
    clockControls,
    clockDisplayPatch,
    deviceSettingsTitle,
    networkInterfaceGroups,
    networkInterfaceName,
    networkInterfaceSummary,
    settingsSections,
    withPendingView,
} from '../static/js/settingsPresentation.mjs';

test('settings sections follow the stable presentation order', () => {
    assert.deepEqual(settingsSections({
        support: {
            sourceNaming: true,
            sync: true,
            bluetoothClear: true,
            language: true,
            clockTime: true,
            systemTimeout: true,
            wifiOnboarding: true,
        },
        clockTime: { utc: 1787882400 },
        systemTimeout: { enabled: true },
        language: { code: 3, options: [{ code: 3, name: 'English' }] },
        sync: { mode: 'SYNC_TO_ROOM' },
        sources: [{ source: 'AUX', displayName: 'Auxiliary' }],
        onboardingUrl: '/setup/',
    }), [
        'clock',
        'standby',
        'language',
        'sync',
        'bluetooth',
        'sources',
        'wifiOnboarding',
    ]);
});

test('connected network paths stay separate from disconnected adapters', () => {
    const wifi = {
        type: 'WIFI_INTERFACE', ssid: 'Test WiFi', ipAddress: '192.0.2.10',
        band: '5GHz', state: 'NETWORK_WIFI_CONNECTED', signalDescription: 'Poor',
    };
    const ethernet = {
        type: 'ETHERNET_INTERFACE', ipAddress: '192.0.2.12', state: 'NETWORK_ETHERNET_CONNECTED',
    };
    const disconnected = {
        type: 'WIFI_INTERFACE', name: 'wlan1', state: 'NETWORK_WIFI_DISCONNECTED',
    };
    const unknown = { type: 'ETHERNET_INTERFACE', name: 'eth1' };
    const network = { interfaces: [disconnected, wifi, unknown, ethernet] };
    assert.deepEqual(networkInterfaceGroups(network), {
        connected: [wifi, ethernet], disconnected: [disconnected, unknown],
    });
    assert.equal(networkInterfaceName(wifi), 'Wi-Fi · Test WiFi');
    assert.equal(networkInterfaceSummary(wifi), '192.0.2.10 · 5GHz');
    assert.equal(networkInterfaceName(ethernet), 'Ethernet');
    assert.equal(networkInterfaceSummary(ethernet), '192.0.2.12');
    assert.equal(networkInterfaceName(disconnected, true), 'Wi-Fi interface · wlan1');
    assert.equal(networkInterfaceSummary(disconnected, true), 'Not connected');
    assert.equal(networkInterfaceSummary(unknown, true), 'Connection state not reported');
    assert.equal(networkInterfaceSummary({}), 'IP not reported');
    assert.deepEqual(networkInterfaceGroups(null), { connected: [], disconnected: [] });
});

test('network visibility does not replace Wi-Fi onboarding', () => {
    assert.deepEqual(settingsSections({ network: { interfaces: [] } }), ['network']);
    assert.deepEqual(settingsSections({ errors: { network: 'Read failed' } }), ['network']);
    assert.deepEqual(settingsSections({
        network: { interfaces: [] }, support: { wifiOnboarding: true }, onboardingUrl: '/setup/',
    }), ['network', 'wifiOnboarding']);
    assert.deepEqual(settingsSections({}), []);
});

test('ST20 clock readback exposes each supported control', () => {
    const snapshot = {
        support: { clockDisplay: true, clockTime: true },
        clockDisplay: {
            enabled: false,
            format: '24',
            timeZone: 'Europe/Prague',
        },
        clockTime: { utc: 1787882400 },
    };

    assert.deepEqual(clockControls(snapshot), {
        display: true,
        format: true,
        timeZone: true,
        currentTime: true,
    });
    assert.deepEqual(clockDisplayPatch(snapshot, 'enabled', true), { enabled: true });
    assert.deepEqual(clockDisplayPatch(snapshot, 'format', '12'), { format: '12' });
    assert.deepEqual(clockDisplayPatch(snapshot, 'timeZone', ' Europe/Berlin '), {
        timeZone: 'Europe/Berlin',
    });
});

test('omitted clock fields do not become controls or writable defaults', () => {
    const snapshot = {
        support: { clockDisplay: true, clockTime: true },
        clockDisplay: { enabled: false },
        clockTime: {},
    };

    assert.deepEqual(clockControls(snapshot), {
        display: true,
        format: false,
        timeZone: false,
        currentTime: false,
    });
    assert.equal(clockDisplayPatch(snapshot, 'format', '24'), null);
    assert.equal(clockDisplayPatch(snapshot, 'timeZone', 'Europe/Prague'), null);
    assert.deepEqual(settingsSections(snapshot), ['clock']);
});

test('clock display toggle requires enabled readback', () => {
    const snapshot = {
        support: { clockDisplay: true },
        clockDisplay: { format: '24', timeZone: 'Europe/Prague' },
    };

    assert.deepEqual(clockControls(snapshot), {
        display: false,
        format: true,
        timeZone: true,
        currentTime: false,
    });
    assert.equal(clockDisplayPatch(snapshot, 'enabled', true), null);
});

test('ST10 clock omission exposes no clock settings', () => {
    const snapshot = { support: {} };

    assert.deepEqual(clockControls(snapshot), {
        display: false,
        format: false,
        timeZone: false,
        currentTime: false,
    });
    assert.deepEqual(settingsSections(snapshot), []);
    assert.equal(clockDisplayPatch(snapshot, 'enabled', true), null);
});

test('unsupported setting data does not expose mutation controls', () => {
    assert.deepEqual(settingsSections({
        support: {},
        clockDisplay: { enabled: true },
        language: { code: 3 },
        sources: [{ source: 'AUX', displayName: 'Auxiliary' }],
    }), []);
});

test('readback-dependent controls stay hidden when only support is reported', () => {
    assert.deepEqual(settingsSections({
        support: { language: true, sync: true, sourceNaming: true, systemTimeout: true },
    }), []);
});

test('automatic standby requires confirmed system-timeout readback', () => {
    assert.deepEqual(settingsSections({
        support: { systemTimeout: true },
        systemTimeout: { enabled: false },
    }), ['standby']);
    assert.deepEqual(settingsSections({
        support: { systemTimeout: true },
    }), []);
});

test('Wi-Fi onboarding requires both device support and a mounted workflow', () => {
    assert.deepEqual(settingsSections({
        support: { wifiOnboarding: true },
        onboardingUrl: '/setup/',
    }), ['wifiOnboarding']);
    assert.deepEqual(settingsSections({
        support: { wifiOnboarding: true },
    }), []);
    assert.deepEqual(settingsSections({
        support: {},
        onboardingUrl: '/setup/',
    }), []);
});

test('partial settings read errors remain visible in order', () => {
    assert.deepEqual(settingsSections({
        support: {},
        errors: {
            language: 'language read failed',
            sources: 'source read failed',
        },
    }), ['language', 'sources']);
});

test('device settings titles keep the target name when using the nested prefix', () => {
    assert.equal(deviceSettingsTitle('Kitchen'), 'Device settings · Kitchen');
    assert.equal(deviceSettingsTitle('Living right', 'right'), 'Device settings · RIGHT · Living right');
    assert.equal(deviceSettingsTitle('Device settings room', 'left', 'Device'), 'Device · LEFT · Device settings room');
});

test('a pending change is shown over the confirmed snapshot until its outcome', () => {
    const snapshot = {
        targetIdentity: 'speaker',
        clockDisplay: { enabled: false, format: '24' },
        sync: { mode: 'SYNC_TO_ROOM' },
    };

    const view = withPendingView(snapshot, { clockDisplay: { enabled: true }, sync: { mode: 'SYNC_TO_ZONE' } });
    assert.deepEqual(view.clockDisplay, { enabled: true, format: '24' });
    assert.equal(view.sync.mode, 'SYNC_TO_ZONE');
    assert.equal(snapshot.clockDisplay.enabled, false, 'the confirmed snapshot must not change');

    assert.equal(withPendingView(snapshot, null), snapshot);
    assert.equal(withPendingView(null, { sync: { mode: 'SYNC_TO_ZONE' } }), null);
    assert.equal(withPendingView(snapshot, { language: { code: 3 } }).language, undefined,
        'a pending value never invents a setting the snapshot lacks');
});
