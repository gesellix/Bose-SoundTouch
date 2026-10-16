import assert from 'node:assert/strict';
import test from 'node:test';
import { sortDeviceEntries } from '../static/js/devicePresentation.js';

import {
    effectiveZoneDetail,
    filterGroupedDeviceEntries,
    physicalMemberMetadata,
    zoneCardPresentation,
    zoneMembershipPresentation,
    zoneMemberCountSummary,
    zoneMemberIdentifiers,
    zoneMemberMetadata,
    zoneTopologyFingerprint,
} from '../static/js/zonePresentation.mjs';

function groupedDevices() {
    return {
        '192.0.2.10': {
            info: { device_id: 'master-id', name: 'Kitchen', ip_address: '192.0.2.10' },
            zone: {
                masterControlId: '192.0.2.10',
                masterDeviceId: 'master-id',
                memberCount: 2,
                members: [
                    { controlId: '192.0.2.10', hwId: 'master-id', deviceIds: ['master-id'] },
                    { controlId: '192.0.2.20', hwId: 'member-id', deviceIds: ['member-id'] },
                ],
            },
        },
        '192.0.2.20': {
            info: { device_id: 'member-id', name: 'Dining', ip_address: '192.0.2.20' },
            zoneMembership: { masterControlId: '192.0.2.10', masterName: 'Kitchen' },
        },
        '192.0.2.2': {
            info: { device_id: 'solo-id', name: 'Atrium', ip_address: '192.0.2.2' },
        },
    };
}

const cardIDs = devices => filterGroupedDeviceEntries(Object.entries(devices)).map(([id]) => id);

test('confirmed groups hide only duplicate logical cards and preserve inventory and sorting', () => {
    const devices = groupedDevices();
    const before = structuredClone(devices);
    const entries = Object.entries(devices);
    const retained = filterGroupedDeviceEntries(entries);

    assert.deepEqual(retained.map(([id]) => id), ['192.0.2.10', '192.0.2.2']);
    assert.equal(retained[0], entries[0]);
    assert.equal(retained[1], entries[2]);
    assert.deepEqual(devices, before);
    assert.deepEqual(entries, Object.entries(before));
    for (const mode of ['ip', 'name']) {
        assert.deepEqual(sortDeviceEntries(retained, mode).map(([id]) => id), [
            '192.0.2.2', '192.0.2.10',
        ]);
    }
    assert.equal(devices['192.0.2.20'].info.name, 'Dining');
});

test('a stereo pair stays one logical group member; physical addresses are not card identities', () => {
    const devices = groupedDevices();
    const zone = devices['192.0.2.10'].zone;
    zone.physicalMemberCount = 3;
    zone.members[1] = {
        ...zone.members[1], kind: 'stereoPair', deviceIds: ['member-id', 'right-id'],
        physicalMembers: [
            { deviceId: 'member-id', ip: '192.0.2.20', role: 'LEFT' },
            { deviceId: 'right-id', ip: '192.0.2.2', role: 'RIGHT' },
        ],
    };
    devices['192.0.2.20'].stereoPair = {
        masterDeviceId: 'member-id',
        members: [{ deviceId: 'member-id' }, { deviceId: 'right-id' }],
    };
    assert.deepEqual(cardIDs(devices), ['192.0.2.10', '192.0.2.2']);
});

test('emitted confirmed topology survives stale, offline and unavailable health metadata', () => {
    for (const connectivity of ['stale', 'offline']) {
        const devices = groupedDevices();
        devices['192.0.2.10'].zone.degraded = true;
        devices['192.0.2.10'].zone.availableMemberCount = 0;
        for (const device of Object.values(devices)) device.status = { connectivity, isConnected: false };
        for (const member of devices['192.0.2.10'].zone.members) member.available = false;
        assert.deepEqual(cardIDs(devices), ['192.0.2.10', '192.0.2.2']);
    }
});

test('a stereo master retains its group and degraded physical member identities', () => {
    const devices = groupedDevices();
    const master = devices['192.0.2.10'];
    master.zone.members[0].deviceIds.push('master-right');
    master.zone.physicalMemberCount = 3;
    master.zone.degraded = true;
    master.stereoPair = {
        masterDeviceId: 'master-id', degraded: true,
        members: [{ deviceId: 'master-id', available: true }, { deviceId: 'master-right', available: false }],
    };
    assert.deepEqual(cardIDs(devices), ['192.0.2.10', '192.0.2.2']);
});

test('incomplete or conflicting projections keep independent card access', async t => {
    const cases = {
        'missing master': devices => { delete devices['192.0.2.10']; },
        'missing zone': devices => { delete devices['192.0.2.10'].zone; },
        'wrong master control': (devices, zone) => { zone.masterControlId = '192.0.2.99'; },
        'wrong master hardware': (devices, zone) => { zone.masterDeviceId = 'other-id'; },
        'unrepresented master': (devices, zone) => {
            zone.members[0] = { controlId: '192.0.2.2', hwId: 'solo-id', deviceIds: ['solo-id'] };
        },
        'no usable master identity': devices => { delete devices['192.0.2.10'].info; },
        'malformed members': (devices, zone) => { zone.members = {}; },
        'null member': (devices, zone) => { zone.members[1] = null; },
        'count mismatch': (devices, zone) => { zone.memberCount = 3; },
        'missing membership': devices => { delete devices['192.0.2.20'].zoneMembership; },
        'wrong membership': devices => { devices['192.0.2.20'].zoneMembership.masterControlId = '192.0.2.2'; },
        'master claims membership': devices => { devices['192.0.2.10'].zoneMembership = { masterControlId: '192.0.2.2' }; },
        'member owns another zone': devices => { devices['192.0.2.20'].zone = {}; },
        'reused control ID': (devices, zone) => { zone.members[1].controlId = '192.0.2.10'; },
        'wrong member hardware': (devices, zone) => { zone.members[1].hwId = 'other-id'; },
        'missing hardware identities': (devices, zone) => { delete zone.members[1].deviceIds; },
        'malformed hardware identities': (devices, zone) => { zone.members[1].deviceIds = {}; },
        'unrepresented member hardware': (devices, zone) => { zone.members[1].deviceIds = ['other-id']; },
        'repeated hardware': (devices, zone) => { zone.members[1].deviceIds.push('member-id'); },
        'shared hardware': (devices, zone) => { zone.members[1].deviceIds.push('master-id'); },
        'stale physical pair identity': (devices, zone) => {
            zone.members[1].deviceIds.push('old-right');
            devices['192.0.2.20'].stereoPair = {
                masterDeviceId: 'member-id', members: [{ deviceId: 'member-id' }, { deviceId: 'new-right' }],
            };
        },
        'unexpected pair projection': (devices, zone) => { zone.members[1].deviceIds.push('right-id'); },
        'mismatched pair master': devices => {
            devices['192.0.2.20'].stereoPair = {
                masterDeviceId: 'other-id', members: [{ deviceId: 'member-id' }],
            };
        },
        'repeated inventory pair identity': (devices, zone) => {
            zone.members[1].deviceIds.push('right-id');
            devices['192.0.2.20'].stereoPair = {
                masterDeviceId: 'member-id', members: [{ deviceId: 'member-id' }, { deviceId: 'member-id' }],
            };
        },
        'duplicate inventory hardware': devices => { devices['192.0.2.2'].info.device_id = 'member-id'; },
        'unregistered member': (devices, zone) => { zone.members[1].controlId = '192.0.2.99'; },
        'conflicting zone control claim': (devices, zone) => {
            devices['192.0.2.2'].zone = { members: [{ ...zone.members[1] }] };
        },
        'conflicting physical hardware claim': devices => {
            devices['192.0.2.2'].zone = { members: [{ controlId: '192.0.2.99', deviceIds: ['member-id'] }] };
        },
    };
    for (const [name, change] of Object.entries(cases)) {
        await t.test(name, () => {
            const devices = groupedDevices();
            change(devices, devices['192.0.2.10'].zone);
            assert.deepEqual(cardIDs(devices), Object.keys(devices));
        });
    }
});

test('confirmed departure and dissolution restore cards on each snapshot without a topology cache', () => {
    const devices = groupedDevices();
    assert.equal(cardIDs(devices).length, 2);
    // The previous group no longer exists, even if membership metadata lags.
    delete devices['192.0.2.10'].zone;
    assert.deepEqual(cardIDs(devices), Object.keys(devices));

    const departed = groupedDevices();
    const zone = departed['192.0.2.10'].zone;
    zone.members[1] = { controlId: '192.0.2.2', hwId: 'solo-id', deviceIds: ['solo-id'] };
    departed['192.0.2.2'].zoneMembership = { masterControlId: '192.0.2.10' };
    delete departed['192.0.2.20'].zoneMembership;
    assert.deepEqual(cardIDs(departed), ['192.0.2.10', '192.0.2.20']);
    assert.deepEqual(cardIDs(groupedDevices()), ['192.0.2.10', '192.0.2.2']);
});

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

test('fresh slave REST role survives an inventory entry without master cache', () => {
    const slave = {
        masterIp: '192.0.2.10',
        masterHwId: 'master-id',
        masterName: 'Kitchen',
        members: [],
        isStandalone: false,
        isMaster: false,
        isSlave: true,
    };

    assert.equal(
        effectiveZoneDetail(slave, null, { status: { zone: null } }, '192.0.2.20'),
        slave,
    );
});

test('fresh slave REST role survives a master projection that has not caught up', () => {
    const projection = {
        masterControlId: '192.0.2.10',
        masterDeviceId: 'master-id',
        physicalMemberCount: 2,
        members: [
            { controlId: '192.0.2.10', deviceIds: ['master-id'], name: 'Kitchen' },
            { controlId: '192.0.2.20', deviceIds: ['member-id'], name: 'Dining' },
        ],
    };
    const joined = {
        masterIp: '192.0.2.10',
        masterHwId: 'master-id',
        masterName: 'Kitchen',
        members: [],
        isStandalone: false,
        isMaster: false,
        isSlave: true,
    };

    assert.equal(effectiveZoneDetail(joined, projection, null, '192.0.2.30'), joined);
    assert.equal(
        effectiveZoneDetail({ isStandalone: true, isMaster: false, isSlave: false }, projection, null, '192.0.2.30'),
        null,
    );
});

test('zone member cards name the group they belong to', () => {
    assert.deepEqual(zoneMembershipPresentation({
        masterControlId: '192.0.2.10',
        masterName: 'Kitchen',
        degraded: false,
    }), {
        label: 'In group · Kitchen',
        title: 'Member of the group led by Kitchen',
        degraded: false,
    });

    const degraded = zoneMembershipPresentation({ masterControlId: '192.0.2.10', degraded: true });
    assert.equal(degraded.label, 'In group · 192.0.2.10');
    assert.equal(degraded.degraded, true);

    assert.equal(zoneMembershipPresentation(null), null);
    assert.equal(zoneMembershipPresentation({ masterName: ' ', masterControlId: '' }), null);
});
