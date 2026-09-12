import assert from 'node:assert/strict';
import test from 'node:test';

import {
    clockControls,
    clockDisplayPatch,
    deviceSettingsTarget,
    deviceSettingsTitle,
    settingsSections,
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

test('standalone settings target keeps its physical control identity', () => {
    const target = deviceSettingsTarget('192.0.2.10', {
        info: { device_id: 'kitchen-id', name: 'Kitchen' },
    });

    assert.deepEqual(target, {
        controlId: '192.0.2.10',
        physicalId: 'kitchen-id',
        name: 'Kitchen',
        role: '',
    });
    assert.equal(deviceSettingsTitle(target.name, target.role), 'Device settings · Kitchen');
});

test('confirmed stereo settings target follows the physical firmware master', () => {
    const device = {
        info: { name: 'Living pair' },
        stereoPair: {
            masterDeviceId: 'right-id',
            members: [
                { deviceId: 'right-id', ipAddress: '192.0.2.11', name: 'Living right', role: 'right' },
                { deviceId: 'left-id', ipAddress: '192.0.2.10', name: 'Living left', role: 'LEFT' },
            ],
        },
    };

    const target = deviceSettingsTarget('192.0.2.11', device);
    assert.deepEqual(target, {
        controlId: '192.0.2.11',
        physicalId: 'right-id',
        name: 'Living right',
        role: 'RIGHT',
    });
    assert.equal(deviceSettingsTitle(target.name, target.role),
        'Device settings · RIGHT · Living right');
});

test('incomplete stereo ownership never falls back to a logical target', () => {
    assert.equal(deviceSettingsTarget('logical-pair', {
        stereoPair: {
            masterDeviceId: 'right-id',
            members: [{ deviceId: 'left-id', ipAddress: '192.0.2.10' }],
        },
    }), null);
});
