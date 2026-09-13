import assert from 'node:assert/strict';
import test from 'node:test';

import {
    connectivityLabel,
    connectivityState,
    currentZoneMember,
    resolvedZoneMember,
    sortDeviceEntries,
    zoneMemberControlID,
} from '../static/js/devicePresentation.js';

test('uses tri-state connectivity before the compatibility flag', () => {
    assert.equal(connectivityState({ status: { connectivity: 'online', isConnected: false } }), 'online');
    assert.equal(connectivityState({ status: { connectivity: 'stale', isConnected: true } }), 'stale');
    assert.equal(connectivityState({ status: { connectivity: 'offline', isConnected: true } }), 'offline');
});

test('falls back to the legacy connected flag and supplies a label', () => {
    assert.equal(connectivityState({ status: { isConnected: true } }), 'online');
    assert.equal(connectivityState({ status: { isConnected: false } }), 'offline');
    assert.equal(connectivityState(undefined), 'offline');
    assert.equal(connectivityLabel({ status: { connectivity: 'stale' } }), 'Stale');
});

test('sorts hostname control IDs by their numeric presentation IP', () => {
    const entries = [
        ['speaker-a', { info: { ip_address: '192.0.2.10' } }],
        ['speaker-b', { info: { ip_address: '192.0.2.2' } }],
    ];

    assert.deepEqual(sortDeviceEntries(entries, 'ip').map(([id]) => id), [
        'speaker-b',
        'speaker-a',
    ]);
});

test('sorts literal IP keys numerically and falls back when an address is missing', () => {
    const entries = [
        ['192.0.2.10', { info: {} }],
        ['192.0.2.2', {}],
        ['192.0.2.1', { info: { ip_address: '' } }],
    ];

    assert.deepEqual(sortDeviceEntries(entries, 'ip').map(([id]) => id), [
        '192.0.2.1',
        '192.0.2.2',
        '192.0.2.10',
    ]);
});

test('uses stable control-ID tie breaks for equal names and addresses', () => {
    const byAddress = [
        ['speaker-b', { info: { ip_address: '192.0.2.4' } }],
        ['speaker-a', { info: { ip_address: '192.0.2.4' } }],
    ];
    const byName = [
        ['speaker-b', { info: { name: ' Kitchen ' } }],
        ['speaker-a', { info: { name: 'kitchen' } }],
    ];

    assert.deepEqual(sortDeviceEntries(byAddress, 'ip').map(([id]) => id), [
        'speaker-a',
        'speaker-b',
    ]);
    assert.deepEqual(sortDeviceEntries(byName, 'name').map(([id]) => id), [
        'speaker-a',
        'speaker-b',
    ]);
});

test('resolves stale zone-detail identities from the current projection', () => {
    const initial = {
        controlId: '192.0.2.20',
        name: 'Living left',
        deviceIds: ['left-id', 'right-id'],
    };
    const current = {
        controlId: 'living.local',
        name: 'Living',
        deviceIds: ['left-id', 'right-id'],
        connectivity: 'online',
    };

    assert.equal(currentZoneMember({ members: [current] }, initial), current);
    assert.deepEqual(resolvedZoneMember({ members: [current] }, initial), {
        member: current,
        controlId: 'living.local',
        name: 'Living',
    });
    assert.equal(zoneMemberControlID({ ip: '192.0.2.99' }), '192.0.2.99');
});
