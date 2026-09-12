import assert from 'node:assert/strict';
import test from 'node:test';

import {
    effectiveZoneDetail,
    physicalMemberMetadata,
    zoneCardPresentation,
    zoneMemberCountSummary,
    zoneMemberIdentifiers,
    zoneMemberMetadata,
    zoneTopologyFingerprint,
} from '../static/js/zonePresentation.mjs';

test('healthy zone cards show one health state and only the logical count', () => {
    assert.deepEqual(zoneCardPresentation({
        memberCount: 2,
        physicalMemberCount: 3,
        availableMemberCount: 2,
        degraded: false,
        members: [
            { physicalMembers: [{ available: true }] },
            { physicalMembers: [{ available: true }, { available: true }] },
        ],
    }), {
        groupLabel: 'Group · 2',
        availabilityLabel: '',
        availabilityTitle: 'All 2 available',
        health: 'healthy',
        healthLabel: 'Healthy group',
    });
});

test('degraded cards report unavailable logical members without healthy N/N copy', () => {
    assert.deepEqual(zoneCardPresentation({
        memberCount: 2,
        physicalMemberCount: 2,
        availableMemberCount: 1,
        degraded: true,
        members: [
            { available: true, physicalMembers: [{ available: true }] },
            { available: false, physicalMembers: [{ available: false }] },
        ],
    }), {
        groupLabel: 'Group · 2',
        availabilityLabel: '1/2 available',
        availabilityTitle: '1 unavailable',
        health: 'degraded',
        healthLabel: 'Degraded group: 1 unavailable',
    });
});

test('degraded stereo zones expose physical loss separately from logical count', () => {
    assert.deepEqual(zoneCardPresentation({
        memberCount: 2,
        physicalMemberCount: 3,
        availableMemberCount: 2,
        degraded: true,
        members: [
            { available: true, physicalMembers: [{ available: true }] },
            {
                available: true,
                physicalMembers: [{ available: true }, { available: false }],
            },
        ],
    }), {
        groupLabel: 'Group · 2',
        availabilityLabel: '2/3 speakers available',
        availabilityTitle: '1 physical speaker unavailable',
        health: 'degraded',
        healthLabel: 'Degraded group: 1 physical speaker unavailable',
    });
});

test('member count summary mentions physical speakers only when counts differ', () => {
    assert.equal(zoneMemberCountSummary(2, 2), '2 members');
    assert.equal(zoneMemberCountSummary(2, 3), '2 members · 3 speakers');
    assert.equal(zoneMemberCountSummary(1, 2), '1 member · 2 speakers');
});

test('logical and physical metadata expose complete accessible identity', () => {
    assert.deepEqual(zoneMemberMetadata({
        controlId: 'living.local',
        ip: '192.0.2.20',
        name: 'Living room',
        model: 'SoundTouch 10',
        kind: 'stereoPair',
        connectivity: 'stale',
    }), {
        connectivity: 'stale',
        connectivityLabel: 'Stale',
        name: 'Living room',
        modelType: 'SoundTouch 10',
        ip: '192.0.2.20',
        kind: 'Stereo pair',
        statusAriaLabel: 'Living room: Stale',
    });

    const physical = physicalMemberMetadata({
        deviceId: 'right-id',
        role: 'right',
        ip: '192.0.2.21',
        name: 'Living right',
        type: 'SoundTouch 10',
        available: false,
        connectivity: 'offline',
    });
    assert.equal(physical.role, 'RIGHT');
    assert.equal(physical.statusAriaLabel, 'Living right: Offline');
    assert.equal(physical.ip, '192.0.2.21');
    assert.equal(physical.modelType, 'SoundTouch 10');
});

test('logical metadata resolves full IP from physical identity for hostname controls', () => {
    const metadata = zoneMemberMetadata({
        controlId: 'living.local',
        ip: 'living.local',
        hwId: 'right-id',
        name: 'Living room',
        kind: 'stereoPair',
        physicalMembers: [
            { deviceId: 'left-id', ip: '192.0.2.20' },
            { deviceId: 'right-id', ip: '192.0.2.21' },
        ],
    });

    assert.equal(metadata.ip, '192.0.2.21');
});

test('zone identity excludes every physical member of an existing stereo pair', () => {
    const ids = zoneMemberIdentifiers({
        controlId: 'living-pair.local',
        hwId: 'left-id',
        deviceIds: ['left-id', 'right-id'],
        physicalMembers: [
            { deviceId: 'left-id', ip: '192.0.2.20' },
            { deviceId: 'right-id', ip: '192.0.2.21' },
        ],
        stereoPair: {
            members: [
                { deviceId: 'left-id', ipAddress: '192.0.2.20' },
                { deviceId: 'right-id', ipAddress: '192.0.2.21' },
            ],
        },
    });

    for (const id of [
        'living-pair.local', 'left-id', 'right-id', '192.0.2.20', '192.0.2.21',
    ]) {
        assert.equal(ids.has(id), true, `missing identity ${id}`);
    }
});

test('topology fingerprint ignores presentation churn and changes with membership', () => {
    const zone = {
        masterControlId: '192.0.2.10',
        masterDeviceId: 'master-id',
        members: [
            {
                controlId: '192.0.2.10',
                hwId: 'master-id',
                deviceIds: ['master-id'],
                name: 'Kitchen',
                connectivity: 'online',
            },
            {
                controlId: '192.0.2.20',
                hwId: 'member-id',
                deviceIds: ['member-id'],
                name: 'Dining',
                connectivity: 'online',
            },
        ],
    };
    const presentationChange = structuredClone(zone);
    presentationChange.members[1].name = 'Breakfast room';
    presentationChange.members[1].connectivity = 'stale';
    const topologyChange = structuredClone(zone);
    topologyChange.members.pop();

    assert.equal(zoneTopologyFingerprint(zone), zoneTopologyFingerprint(presentationChange));
    assert.notEqual(zoneTopologyFingerprint(zone), zoneTopologyFingerprint(topologyChange));
    assert.equal(zoneTopologyFingerprint(null), '');
});

test('live projection replaces stale REST roles for master and member details', () => {
    const projection = {
        masterControlId: '192.0.2.10',
        masterDeviceId: 'master-id',
        physicalMemberCount: 2,
        members: [
            { controlId: '192.0.2.10', deviceIds: ['master-id'], name: 'Kitchen' },
            { controlId: '192.0.2.20', deviceIds: ['member-id'], name: 'Dining' },
        ],
    };
    const staleStandalone = { isStandalone: true, isMaster: false, isSlave: false };

    const master = effectiveZoneDetail(staleStandalone, projection, {}, '192.0.2.10');
    assert.equal(master.isMaster, true);
    assert.equal(master.isSlave, false);
    assert.equal(master.members.length, 1);

    const member = effectiveZoneDetail(staleStandalone, projection, null, '192.0.2.20');
    assert.equal(member.isMaster, false);
    assert.equal(member.isSlave, true);
    assert.equal(member.masterName, 'Kitchen');
    assert.equal(effectiveZoneDetail(staleStandalone, projection, null, '192.0.2.99'), null);
});

test('standalone inventory projection replaces stale REST zone management', () => {
    const staleZone = { isStandalone: false, isMaster: true, isSlave: false };
    const standalone = effectiveZoneDetail(staleZone, null, { status: {} }, '192.0.2.10');

    assert.equal(standalone.isStandalone, true);
    assert.equal(standalone.isMaster, false);
    assert.deepEqual(standalone.members, []);
    assert.equal(effectiveZoneDetail(staleZone, null, null, '192.0.2.20'), staleZone);
});
