import assert from 'node:assert/strict';
import test from 'node:test';
import { bassControl, pairBassControl, balanceControl, balanceLabel, physicalSettingsTargets, settingsBassReadback, writeSoundSetting } from '../static/js/soundSettingsPresentation.mjs';
import { createSteppedInteraction } from '../static/js/steppedInteraction.mjs';

const physical = (controlId, physicalId, role = '') => ({ controlId, physicalId, role, name: physicalId,
    status: { epoch: 1, revision: 2, connectivity: 'online', bass: { ActualBass: 0 }, ...(role ? { group: group() } : {}) } });
const capabilities = { BassAvailable: true, BassMin: -9, BassMax: 0, BassDefault: 0 };
const group = (master = 'right') => ({ ID: 'pair', MasterDeviceID: master,
    Roles: { Roles: [{ DeviceID: 'right', Role: 'RIGHT' }, { DeviceID: 'left', Role: 'LEFT' }] } });
const pairDevice = () => ({ stereoPair: { id: 'pair', masterDeviceId: 'right',
    members: [{ deviceId: 'right', role: 'RIGHT' }, { deviceId: 'left', role: 'LEFT' }] },
    settingsTargets: [physical('right.local', 'right', 'RIGHT'), physical('left.local', 'left', 'LEFT')],
    status: { epoch: 1, group: group(), balance: { Available: true, Min: -7, Max: 7, Default: 0, Actual: 3 } } });

test('bass uses reported capabilities and current value without a fallback range', () => {
    const target = physical('speaker.local', 'speaker');
    assert.equal(bassControl(target), null);
    assert.deepEqual(bassControl({ ...target, bassCapabilities: capabilities }), { min: -9, max: 0, defaultValue: 0, value: 0 });
    for (const caps of [{ ...capabilities, BassAvailable: false }, { ...capabilities, BassMax: undefined },
        { ...capabilities, BassMin: 1 }, { ...capabilities, BassDefault: 9 }]) {
        assert.equal(bassControl({ ...target, bassCapabilities: caps }), null);
    }
    assert.equal(bassControl({ ...target, status: { bass: {} }, bassCapabilities: capabilities }), null);
});

function bassPair(master = 'right') {
    const device = pairDevice();
    device.stereoPair.masterDeviceId = master;
    device.status.group = group(master);
    device.settingsTargets = device.settingsTargets.map(target => ({ ...target, bassCapabilities: capabilities,
        status: { ...target.status, group: group(master) } }));
    return device;
}

test('pair bass follows either master and requires matching capabilities from both coherent members', () => {
    for (const master of ['left', 'right']) {
        const device = bassPair(master);
        assert.equal(pairBassControl(`${master}.local`, device).controlId, `${master}.local`);
        const changed = edit => ({ ...device, settingsTargets: device.settingsTargets.map((target, index) => index ? edit(target) : target) });
        for (const invalid of [
            { ...device, settingsTargets: [device.settingsTargets[0]] },
            { ...device, settingsTargets: [...device.settingsTargets, device.settingsTargets[0]] },
            changed(target => ({ ...target, bassCapabilities: { ...capabilities, BassMax: 1 } })),
            changed(target => ({ ...target, status: { ...target.status, connectivity: 'offline' } })),
            changed(target => ({ ...target, status: { ...target.status, group: {} } })),
        ]) assert.equal(pairBassControl(`${master}.local`, invalid), null);
        const divergent = changed(target => ({ ...target, status: { ...target.status, bass: { ActualBass: -1 } } }));
        assert.equal(pairBassControl(`${master}.local`, divergent).observationConfirmed, false);
        device.status.balance = null;
        assert.ok(pairBassControl(`${master}.local`, device), 'Pair bass must not depend on balance support');
    }
});

test('pair bass writes only master once and verifies both members plus unchanged pair topology', async () => {
    const device = bassPair();
    const target = { ...pairBassControl('right.local', device), kind: 'bass' };
    let posts = 0, reads = [], topologyReads = 0, fail = false, actual = -1, epoch = 1, topology = device;
    const api = {
        bass: async (id, value) => { assert.equal(id, 'right.local'); assert.equal(value, -1); posts++; return { success: true }; },
        settings: async id => {
            reads.push(id);
            return { success: true, data: { targetIdentity: id.split('.')[0], statusEpoch: id === 'left.local' ? epoch : 1, statusRevision: 2,
                bassCapabilities: capabilities, bass: { ActualBass: id === 'left.local' ? actual : -1 }, errors: fail ? { bass: 'offline' } : {} } };
        },
        device: async () => { topologyReads++; return { success: true, data: topology }; },
    };
    assert.deepEqual(await writeSoundSetting(api, target, -1), { success: true, confirmedValue: -1 });
    assert.deepEqual(reads.sort(), ['left.local', 'right.local']); assert.equal(topologyReads, 1); assert.equal(posts, 1);
    actual = 0;
    assert.equal((await writeSoundSetting(api, target, -1)).outcome, 'unverified');
    actual = -1; epoch = 2;
    assert.equal((await writeSoundSetting(api, target, -1)).outcome, 'unverified');
    epoch = 1; fail = true;
    assert.equal((await writeSoundSetting(api, target, -1)).outcome, 'unverified');
    fail = false;
    for (const invalid of [
        { ...device, settingsTargets: device.settingsTargets.map(member => ({ ...member, status: { ...member.status, revision: 3, bass: { ActualBass: -2 } } })) },
        { ...device, stereoPair: null },
        { ...device, status: { ...device.status, group: group('left') } },
        { ...device, settingsTargets: device.settingsTargets.map(member => ({ ...member, status: { ...member.status, epoch: 2 } })) },
    ]) {
        topology = invalid;
        assert.equal((await writeSoundSetting(api, target, -1)).outcome, 'unverified');
    }
    assert.equal(posts, 8, 'Each operation gets one write, without fanout or replay');
    topology = device;
    api.bass = async () => { posts++; throw new Error('Response lost'); };
    assert.deepEqual(await writeSoundSetting(api, target, -1), { success: true, confirmedValue: -1 });
    const before = reads.length;
    api.bass = async () => { posts++; throw Object.assign(new Error('Rejected'), { definitive: true }); };
    assert.equal((await writeSoundSetting(api, target, -1)).error, 'Rejected');
    assert.equal(reads.length, before, 'Definitive rejection must skip confirmation reads');
});

test('one pair balance follows a confirmed RIGHT master and device groups order LEFT then RIGHT', () => {
    const left = physical('left.local', 'left', 'LEFT');
    const right = physical('right.local', 'right', 'RIGHT');
    const device = { stereoPair: { id: 'pair', masterDeviceId: 'right', members: [right, left].map(target => ({ deviceId: target.physicalId, role: target.role })) },
        settingsTargets: [right, left], status: { epoch: 1, group: group(), balance: { Available: true, Min: -7, Max: 7, Default: 0, Actual: 3 } } };
    assert.deepEqual(physicalSettingsTargets('right.local', device), [left, right]);
    assert.equal(balanceControl('right.local', device).controlId, 'right.local');
    assert.equal(balanceControl('right.local', { ...device, settingsTargets: [left] }), null);
    assert.equal(balanceControl('right.local', { ...device, settingsTargets: [left, right, { ...right }] }), null);
});

test('accepted writes get one confirmation read, and a failed read never replays the POST', async () => {
    let posts = 0;
    let gets = 0;
    const api = { bass: async () => { posts += 1; return { success: true }; },
        settings: async () => { gets += 1; throw new Error('offline'); } };
    const target = { ...physical('speaker.local', 'speaker'), kind: 'bass' };
    assert.match((await writeSoundSetting(api, target, -1)).error, /Unverified/);
    assert.equal(posts, 1);
    assert.equal(gets, 1);
    api.settings = async () => ({ success: true, data: { targetIdentity: 'speaker', statusEpoch: 1, bassCapabilities: capabilities, bass: { ActualBass: -1 } } });
    assert.deepEqual(await writeSoundSetting(api, target, -1), { success: true, confirmedValue: -1 });
    api.settings = async () => ({ success: true, data: { targetIdentity: 'replacement', bass: { ActualBass: -1 } } });
    assert.match((await writeSoundSetting(api, target, -1)).error, /Unverified/);
});

test('balance uses coherent group membership and tolerates identical reordered refreshes', () => {
    const device = pairDevice();
    assert.equal(balanceControl('right.local', device).value, 3);
    const reordered = { ...group(), Roles: { Roles: group().Roles.Roles.toReversed() } };
    assert.equal(balanceControl('right.local', { ...device, status: { ...device.status, group: reordered } }).value, 3);
    for (const invalid of [null, {}, group('left'), { ...group(), ID: 'replacement' },
        { ...group(), Roles: { Roles: [{ DeviceID: 'right', Role: 'LEFT' }, { DeviceID: 'right', Role: 'RIGHT' }] } }]) {
        assert.equal(balanceControl('right.local', { ...device, status: { ...device.status, group: invalid } }), null);
    }
    assert.equal(balanceLabel(0), 'Centered');
});

test('balance confirmation requires available reported control and same physical pair', async () => {
    const device = pairDevice();
    const target = { ...balanceControl('right.local', device), kind: 'balance' };
    let reads = 0;
    let writes = 0;
    let readback = device;
    const api = { balance: async () => { writes++; return { success: true }; },
        device: async () => { reads++; return { success: true, data: readback }; } };
    assert.deepEqual(await writeSoundSetting(api, target, 3), { success: true, confirmedValue: 3 });
    for (const balance of [{ ...device.status.balance, Available: false }, { ...device.status.balance, Max: 2 },
        { ...device.status.balance, Default: undefined }]) {
        readback = { ...device, status: { ...device.status, balance } };
        assert.match((await writeSoundSetting(api, target, 3)).error, /Unverified/);
    }
    readback = { ...device, status: { ...device.status, group: {} } };
    assert.match((await writeSoundSetting(api, target, 3)).error, /Unverified/);
    assert.equal(writes, 5);
    assert.equal(reads, 5);
});

test('fresh settings bass beats equal or older cache and yields to a later physical observation', () => {
    const bass = { ActualBass: -2 };
    const snapshot = { statusEpoch: 1, statusRevision: 4, bass };
    assert.equal(settingsBassReadback({ epoch: 1, revision: 4, bass: { ActualBass: 0 } }, snapshot), bass);
    assert.equal(settingsBassReadback({ epoch: 1, revision: 3, bass: { ActualBass: 0 } }, snapshot), bass);
    const updated = { epoch: 1, revision: 5, bass: { ActualBass: -3 } };
    assert.equal(settingsBassReadback(updated, snapshot), updated.bass);
    assert.equal(settingsBassReadback(updated, null), updated.bass);
});

function deferred() {
    let resolve;
    const promise = new Promise(done => { resolve = done; });
    return { promise, resolve };
}
const target = (key = 'A', projection = '1') => ({ key, projection, value: 0, min: -9, max: 0, disabled: false });
const flush = async () => { await Promise.resolve(); await Promise.resolve(); };

test('master-first pair events retain the latest queued choice and cannot clear pair uncertainty', async () => {
    const first = deferred();
    const writes = [];
    const state = createSteppedInteraction({ send: (_, value) => {
        writes.push(value);
        return writes.length === 1 ? first.promise : { success: false, outcome: 'unverified', error: 'Pair unverified' };
    } });
    const pair = (master, peer) => ({ ...target('pair', JSON.stringify([master, peer])), value: master,
        observationConfirmed: master === peer });
    state.update(pair(0, 0));
    state.choose(-1); state.choose(-2); state.choose(-3);
    state.update(pair(-1, 0));
    first.resolve({ success: true, confirmedValue: -1 });
    for (let i = 0; i < 6; i++) await flush();
    assert.deepEqual(writes, [-1, -3], 'A temporary peer lag must not cancel the newest queued value');
    assert.equal(state.view().failure, 'Pair unverified');
    state.update(pair(-3, -1));
    assert.equal(state.view().failure, 'Pair unverified', 'Master-only convergence cannot confirm the pair');
    state.update(pair(-3, -3));
    assert.equal(state.view().failure, '');
    assert.equal(state.view().confirmedValue, -3);
    state.dispose();
});

test('one discrete click writes once, unchanged values and passive updates never write', async () => {
    const requests = [];
    const state = createSteppedInteraction({ interval: 0, send: (_, value) => { requests.push(value); return { success: true, confirmedValue: value }; } });
    state.update(target());
    state.choose(0);
    state.update(target());
    assert.deepEqual(requests, []);
    state.choose(-1);
    await flush();
    state.choose(-1);
    state.update(target());
    assert.deepEqual(requests, [-1]);
    state.dispose();
});

test('latest final failure persists across unchanged renders and clears only on a later projection', async () => {
    const pending = deferred();
    const state = createSteppedInteraction({ interval: 0, send: () => pending.promise });
    state.update(target());
    state.choose(-1);
    state.update(target('A', '2')); // newer projection arrives before the failure
    pending.resolve({ success: false, error: 'Final write failed' });
    await flush();
    state.update(target('A', '2'));
    assert.equal(state.view().failure, 'Final write failed');
    state.update(target('A', '3'));
    assert.equal(state.view().failure, '');
    state.dispose();
});

test('delayed A response cannot settle into A after B, and newer projection wins over old readback', async () => {
    const pending = deferred();
    const state = createSteppedInteraction({ interval: 0, send: () => pending.promise });
    state.update(target());
    state.choose(-1);
    state.update(target('B'));
    state.update(target('A'));
    pending.resolve({ success: false, error: 'Old A failed' });
    await flush();
    assert.equal(state.view().failure, '');
    assert.equal(state.view().value, 0);
    state.dispose();

    const readback = deferred();
    const newer = createSteppedInteraction({ interval: 0, send: () => readback.promise });
    newer.update(target());
    newer.choose(-1);
    newer.update({ ...target('A', '2'), value: -3 });
    readback.resolve({ success: true, confirmedValue: -1 });
    await flush();
    assert.equal(newer.view().value, -3);
    assert.equal(newer.view().confirmedValue, null);
    newer.dispose();
});


test('a confirmation read from another connection epoch cannot confirm the admitted write', async () => {
    let writes = 0;
    const target = { ...physical('speaker.local', 'speaker'), kind: 'bass' };
    const api = { bass: async () => { writes++; return { success: true }; },
        settings: async () => ({ success: true, data: { targetIdentity: 'speaker', statusEpoch: 2,
            bassCapabilities: capabilities, bass: { ActualBass: -1 } } }) };
    assert.equal((await writeSoundSetting(api, target, -1)).outcome, 'unverified');
    assert.equal(writes, 1);
    const device = pairDevice();
    const balanceTarget = { ...balanceControl('right.local', device), kind: 'balance' };
    const balanceApi = { balance: async () => { writes++; return { success: true }; },
        device: async () => ({ success: true, data: { ...device, status: { ...device.status, epoch: 2 } } }) };
    assert.equal((await writeSoundSetting(balanceApi, balanceTarget, 3)).outcome, 'unverified');
    assert.equal(writes, 2);
});


test('contradictory physical member groups disable logical pair balance', () => {
    const device = pairDevice();
    assert.ok(balanceControl('right.local', device));
    for (const contradictory of [null, {}, group('left'), { ...group(), ID: 'other-pair' }]) {
        const targets = device.settingsTargets.map(target => target.role === 'LEFT'
            ? { ...target, status: { ...target.status, group: contradictory } } : target);
        assert.equal(balanceControl('right.local', { ...device, settingsTargets: targets }), null);
    }
});


test('ordinary settings retain the existing trimmed hardware identity compatibility', () => {
    const target = physical('speaker.local', 'speaker');
    assert.deepEqual(physicalSettingsTargets('speaker.local', { info: { device_id: ' speaker ' },
        settingsTargets: [target] }), [target]);
});

test('an ambiguous lost mutation response receives one read and never another write', async () => {
    let writes = 0, reads = 0, actual = -1;
    const target = { ...physical('speaker.local', 'speaker'), kind: 'bass' };
    const api = { bass: async () => { writes++; throw new Error('Response lost'); },
        settings: async () => { reads++; return { success: true, data: { targetIdentity: 'speaker', statusEpoch: 1,
            bassCapabilities: capabilities, bass: { ActualBass: actual } } }; } };
    assert.deepEqual(await writeSoundSetting(api, target, -1), { success: true, confirmedValue: -1 });
    assert.equal(writes, 1); assert.equal(reads, 1);
    actual = 0;
    assert.equal((await writeSoundSetting(api, target, -1)).outcome, 'unverified');
    assert.equal(writes, 2); assert.equal(reads, 2);
    api.bass = async () => { writes++; return { success: false, error: 'Rejected' }; };
    assert.equal((await writeSoundSetting(api, target, -1)).error, 'Rejected');
    assert.equal(writes, 3); assert.equal(reads, 2);
});


test('checked sound writes skip readback only for a definitive rejection', async () => {
    const target = { ...physical('speaker.local', 'speaker'), kind: 'bass' };
    let writes = 0, reads = 0, definitive = true;
    const api = { bass: () => { throw new Error('Plain mutation should not be used'); },
        bassChecked: async () => { writes++; throw Object.assign(new Error('Rejected or uncertain'), { definitive }); },
        settings: async () => { reads++; return { success: true, data: { targetIdentity: 'speaker', statusEpoch: 1,
            bassCapabilities: capabilities, bass: { ActualBass: -1 } } }; } };
    assert.equal((await writeSoundSetting(api, target, -1)).error, 'Rejected or uncertain');
    assert.equal(reads, 0); assert.equal(writes, 1);
    definitive = false;
    assert.deepEqual(await writeSoundSetting(api, target, -1), { success: true, confirmedValue: -1 });
    assert.equal(reads, 1); assert.equal(writes, 2);
});
