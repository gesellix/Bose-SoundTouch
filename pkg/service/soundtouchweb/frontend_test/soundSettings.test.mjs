import assert from 'node:assert/strict';
import test from 'node:test';
import { bassControl, pairBassControl, balanceControl, balanceLabel, physicalSettingsTargets, bassObservations, writeSoundSetting } from '../static/js/soundSettingsPresentation.mjs';
import { createSteppedInteraction, observationsAdvanced } from '../static/js/steppedInteraction.mjs';

async function writeOutcome(...args) {
    const { observations: _observations, ...outcome } = await writeSoundSetting(...args);
    return outcome;
}

const physical = (controlId, physicalId, role = '') => ({ controlId, physicalId, role, name: physicalId,
    status: { epoch: 1, revision: 2, bassRevision: 1, connectivity: 'online', bass: { ActualBass: 0 } } });
const capabilities = { BassAvailable: true, BassMin: -9, BassMax: 0, BassDefault: 0 };
const group = (master = 'right') => ({ ID: 'pair', MasterDeviceID: master,
    Roles: { Roles: [{ DeviceID: 'right', Role: 'RIGHT' }, { DeviceID: 'left', Role: 'LEFT' }] } });
const pairDevice = () => ({ info: { device_id: 'right' }, stereoPair: { id: 'pair', masterDeviceId: 'right',
    members: [{ deviceId: 'right', role: 'RIGHT' }, { deviceId: 'left', role: 'LEFT' }] },
    settingsTargets: [physical('right.local', 'right', 'RIGHT'), physical('left.local', 'left', 'LEFT')],
    status: { epoch: 1, group: group(), balance: { Available: true, Min: -7, Max: 7, Default: 0, Actual: 3 } } });

test('bass uses reported capabilities and current value without a fallback range', () => {
    const target = physical('speaker.local', 'speaker');
    assert.equal(bassControl(target), null);
    assert.deepEqual(bassControl({ ...target, status: { ...target.status, bassCapabilities: capabilities } }), { min: -9, max: 0, defaultValue: 0, value: 0 });
    for (const caps of [{ ...capabilities, BassAvailable: false }, { ...capabilities, BassMax: undefined },
        { ...capabilities, BassMin: 1 }, { ...capabilities, BassDefault: 9 }]) {
        assert.equal(bassControl({ ...target, status: { ...target.status, bassCapabilities: caps } }), null);
    }
    assert.equal(bassControl({ ...target, status: { bass: {}, bassCapabilities: capabilities } }), null);
    assert.equal(bassControl({ ...target, bassCapabilities: capabilities }), null, 'Legacy snapshot capabilities cannot enable a control');
    assert.equal(bassControl({ ...target, status: { ...target.status,
        bassCapabilities: { ...capabilities, DeviceID: 'replacement' } } }), null);
});

function bassPair(master = 'right') {
    const device = pairDevice();
    device.stereoPair.masterDeviceId = master;
    device.status.group = group(master);
    device.info.device_id = master;
    device.settingsTargets = device.settingsTargets.map(target => ({ ...target,
        status: { ...target.status, bassCapabilities: capabilities } }));
    return device;
}

test('pair bass follows either projected master and requires matching capabilities from both members', () => {
    for (const master of ['left', 'right']) {
        const device = bassPair(master);
        assert.equal(pairBassControl(`${master}.local`, device).controlId, `${master}.local`);
        const changed = edit => ({ ...device, settingsTargets: device.settingsTargets.map((target, index) => index ? edit(target) : target) });
        for (const invalid of [
            { ...device, settingsTargets: [device.settingsTargets[0]] },
            { ...device, settingsTargets: [...device.settingsTargets, device.settingsTargets[0]] },
            changed(target => ({ ...target, status: { ...target.status, bassCapabilities: { ...capabilities, BassMax: 1 } } })),
            changed(target => ({ ...target, status: { ...target.status, connectivity: 'offline' } })),
        ]) assert.equal(pairBassControl(`${master}.local`, invalid), null);
        const divergent = changed(target => ({ ...target, status: { ...target.status, bass: { ActualBass: -1 } } }));
        assert.equal(pairBassControl(`${master}.local`, divergent).observationConfirmed, false);
        device.status.balance = null;
        assert.ok(pairBassControl(`${master}.local`, device), 'Pair bass must not depend on balance support');
    }
});

test('pair bass writes only master once and requires fresh values from both members of the same pair', async () => {
    const device = bassPair();
    const target = { ...pairBassControl('right.local', device), kind: 'bass' };
    const fresh = () => ({ ...device, settingsTargets: device.settingsTargets.map(member => ({ ...member,
        status: { ...member.status, bassRevision: 2, bass: { ActualBass: -1 } } })) });
    let posts = 0, reads = [], settingsReads = 0, readback = fresh();
    const api = {
        bassChecked: async (id, value) => { assert.equal(id, 'right.local'); assert.equal(value, -1); posts++; },
        settings: async () => { settingsReads++; throw new Error('Unexpected settings read'); },
        device: async id => { reads.push(id); return { success: true, data: readback }; },
    };
    assert.deepEqual(await writeOutcome(api, target, -1), { success: true, confirmedValue: -1 });
    assert.deepEqual(reads, ['right.local']); assert.equal(posts, 1);
    const changed = edit => ({ ...fresh(), settingsTargets: fresh().settingsTargets.map((member, index) => index ? edit(member) : member) });
    for (const invalid of [
        changed(member => ({ ...member, status: { ...member.status, bass: { ActualBass: 0 } } })),
        changed(member => ({ ...member, status: { ...member.status, bassRevision: 1, revision: 99, volume: { ActualVolume: 40 } } })),
        changed(member => ({ ...member, status: { ...member.status, bassRevision: undefined } })),
        changed(member => ({ ...member, status: { ...member.status, connectivity: 'offline' } })),
        changed(member => ({ ...member, status: { ...member.status, epoch: 2 } })),
        changed(member => ({ ...member, physicalId: 'replacement' })),
        { ...fresh(), stereoPair: null },
        { ...fresh(), stereoPair: { ...device.stereoPair, id: 'replacement' } },
        { ...fresh(), stereoPair: { ...device.stereoPair, masterDeviceId: 'left' } },
    ]) {
        readback = invalid;
        assert.equal((await writeOutcome(api, target, -1)).outcome, 'unverified');
    }
    assert.equal(posts, 10, 'Each operation gets one write, without fanout or replay');
    assert.equal(reads.length, posts, 'Each operation reads only the projected master');
    readback = fresh();
    api.bassChecked = async () => { posts++; throw new Error('Response lost'); };
    assert.deepEqual(await writeOutcome(api, target, -1), { success: true, confirmedValue: -1 });
    const before = reads.length;
    api.bassChecked = async () => { posts++; throw Object.assign(new Error('Rejected'), { definitive: true }); };
    assert.equal((await writeOutcome(api, target, -1)).error, 'Rejected');
    assert.equal(reads.length, before, 'Definitive rejection must skip confirmation reads');
    assert.equal(settingsReads, 0);
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
    // The projection lists only registered members, and a balance write needs
    // only the master's WebSocket, so an unregistered peer must not remove it.
    const masterOnly = { ...device, settingsTargets: [right] };
    assert.equal(balanceControl('right.local', masterOnly).controlId, 'right.local');
    assert.equal(pairBassControl('right.local', masterOnly), null);
});

test('accepted writes get one confirmation read, and a failed read never replays the POST', async () => {
    let posts = 0;
    let gets = 0;
    const api = { bassChecked: async () => { posts += 1; },
        device: async () => { gets += 1; throw new Error('offline'); } };
    const target = { ...physical('speaker.local', 'speaker'), kind: 'bass' };
    assert.match((await writeOutcome(api, target, -1)).error, /Unverified/);
    assert.equal(posts, 1);
    assert.equal(gets, 1);
    api.device = async () => ({ success: true, data: { info: { device_id: 'speaker' }, status: {
        ...target.status, bassRevision: 2, bassCapabilities: capabilities, bass: { ActualBass: -1 } } } });
    assert.deepEqual(await writeOutcome(api, target, -1), { success: true, confirmedValue: -1 });
    api.device = async () => ({ success: true, data: { info: { device_id: 'replacement' }, status: {
        ...target.status, bassRevision: 2, bassCapabilities: capabilities, bass: { ActualBass: -1 } } } });
    assert.match((await writeOutcome(api, target, -1)).error, /Unverified/);
});

test('balance trusts server pair projection and validates its reported range', () => {
    const device = pairDevice();
    assert.equal(balanceControl('right.local', device).value, 3);
    assert.equal(balanceControl('right.local', { ...device, status: { ...device.status, group: null } }).value, 3);
    assert.equal(balanceControl('left.local', device), null);
    assert.equal(balanceControl('right.local', { ...device, stereoPair: null }), null);
    assert.equal(balanceLabel(0), 'Centered');
});

test('balance confirmation requires available reported control and same physical pair', async () => {
    const device = pairDevice();
    const target = { ...balanceControl('right.local', device), kind: 'balance' };
    let reads = 0;
    let writes = 0;
    let readback = device;
    const api = { balanceChecked: async () => { writes++; },
        device: async () => { reads++; return { success: true, data: readback }; } };
    assert.deepEqual(await writeOutcome(api, target, 3), { success: true, confirmedValue: 3 });
    for (const balance of [{ ...device.status.balance, Available: false }, { ...device.status.balance, Max: 2 },
        { ...device.status.balance, Default: undefined }]) {
        readback = { ...device, status: { ...device.status, balance } };
        assert.match((await writeOutcome(api, target, 3)).error, /Unverified/);
    }
    readback = { ...device, stereoPair: { ...device.stereoPair, id: 'replacement' } };
    assert.match((await writeOutcome(api, target, 3)).error, /Unverified/);
    assert.equal(writes, 5);
    assert.equal(reads, 5);
});

test('bass confirmation requires every physical observation to advance within its connection', () => {
    const before = bassObservations(pairBassControl('right.local', bassPair()));
    const after = before.map(row => [...row.slice(0, 3), row[3] + 1]);
    assert.ok(observationsAdvanced(after, before));
    for (const invalid of [undefined, [], before, after.slice(1), [...after, after[0]],
        [after[0], before[1]], [after[0], [...after[1].slice(0, 3), undefined]],
        [after[0], ['replacement.local', ...after[1].slice(1)]],
        [after[0], [after[1][0], 'replacement', ...after[1].slice(2)]],
        [after[0], [after[1][0], after[1][1], 2, after[1][3]]]]) {
        assert.equal(observationsAdvanced(invalid, before), false);
    }
    for (const invalid of [undefined, [], [[1]], before.map(row => [...row.slice(0, 3), undefined])]) {
        assert.equal(observationsAdvanced(after, invalid), false);
    }
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
    const pair = (master, peer, masterToken, peerToken) => ({
        ...target('pair', JSON.stringify([master, peer, masterToken, peerToken])), value: master,
        observations: [['right.local', 'right', 1, masterToken], ['left.local', 'left', 1, peerToken]],
        observationConfirmed: master === peer });
    state.update(pair(0, 0, 1, 1));
    state.choose(-1); state.choose(-2); state.choose(-3);
    state.update(pair(-1, 0, 2, 1));
    first.resolve({ success: true, confirmedValue: -1 });
    for (let i = 0; i < 6; i++) await flush();
    assert.deepEqual(writes, [-1, -3], 'A temporary peer lag must not cancel the newest queued value');
    assert.equal(state.view().failure, 'Pair unverified');
    state.update(pair(-3, -1, 3, 2));
    assert.equal(state.view().failure, 'Pair unverified', 'Master-only convergence cannot confirm the pair');
    state.update(pair(-3, -3, 3, 3));
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
    const api = { bassChecked: async () => { writes++; },
        device: async () => ({ success: true, data: { info: { device_id: 'speaker' }, status: {
            epoch: 2, bassRevision: 2, bassCapabilities: capabilities, bass: { ActualBass: -1 } } } }) };
    assert.equal((await writeOutcome(api, target, -1)).outcome, 'unverified');
    assert.equal(writes, 1);
    const device = pairDevice();
    const balanceTarget = { ...balanceControl('right.local', device), kind: 'balance' };
    const balanceApi = { balanceChecked: async () => { writes++; },
        device: async () => ({ success: true, data: { ...device, status: { ...device.status, epoch: 2 } } }) };
    assert.equal((await writeOutcome(balanceApi, balanceTarget, 3)).outcome, 'unverified');
    assert.equal(writes, 2);
});


test('standalone aliases and missing Info retain an ordinary settings target', () => {
    const status = physical('speaker.local', 'speaker').status;
    for (const id of ['speaker.local', '192.0.2.1']) {
        assert.deepEqual(physicalSettingsTargets(id, { info: { device_id: ' speaker ' }, status }),
            [{ controlId: id, physicalId: 'speaker', name: id, role: '', status }]);
        assert.deepEqual(physicalSettingsTargets(id, { status }),
            [{ controlId: id, physicalId: '', name: id, role: '', status }]);
    }
});

test('an ambiguous lost mutation response receives one read and never another write', async () => {
    let writes = 0, reads = 0, actual = -1;
    const target = { ...physical('speaker.local', 'speaker'), kind: 'bass' };
    const api = { bassChecked: async () => { writes++; throw new Error('Response lost'); },
        device: async () => { reads++; return { success: true, data: { info: { device_id: 'speaker' }, status: {
            ...target.status, bassRevision: 2, bassCapabilities: capabilities, bass: { ActualBass: actual } } } }; } };
    assert.deepEqual(await writeOutcome(api, target, -1), { success: true, confirmedValue: -1 });
    assert.equal(writes, 1); assert.equal(reads, 1);
    actual = 0;
    assert.equal((await writeOutcome(api, target, -1)).outcome, 'unverified');
    assert.equal(writes, 2); assert.equal(reads, 2);
    api.bassChecked = async () => { writes++; throw Object.assign(new Error('Rejected'), { definitive: true }); };
    assert.equal((await writeOutcome(api, target, -1)).error, 'Rejected');
    assert.equal(writes, 3); assert.equal(reads, 2);
});


test('checked bass and balance errors skip readback only for definitive rejection', async () => {
    for (const kind of ['bass', 'balance']) {
        const device = pairDevice();
        const target = { ...(kind === 'bass' ? physical('speaker.local', 'speaker')
            : balanceControl('right.local', device)), kind };
        const value = kind === 'bass' ? -1 : 3;
        let writes = 0, reads = 0, unchecked = 0, settingsReads = 0, definitive = true;
        const api = { [kind]: () => { unchecked++; throw new Error('Unchecked mutation'); },
            [`${kind}Checked`]: async () => { writes++; throw Object.assign(new Error('Rejected or uncertain'), { definitive }); },
            settings: async () => { settingsReads++; },
            device: async () => { reads++; return { success: true, data: kind === 'balance' ? device : {
                info: { device_id: 'speaker' }, status: { ...target.status, bassRevision: 2,
                    bassCapabilities: capabilities, bass: { ActualBass: value } } } }; } };
        assert.equal((await writeOutcome(api, target, value)).error, 'Rejected or uncertain');
        assert.equal(reads, 0); assert.equal(writes, 1);
        definitive = false;
        assert.deepEqual(await writeOutcome(api, target, value), { success: true, confirmedValue: value });
        assert.equal(reads, 1); assert.equal(writes, 2);
        assert.equal(unchecked, 0); assert.equal(settingsReads, 0);
    }
});

test('queued writes retain bass evidence from earlier direct readbacks before a push arrives', async () => {
    const first = deferred();
    const floors = [];
    const original = [['right.local', 'right', 1, 1], ['left.local', 'left', 1, 1]];
    const observed = original.map(row => [...row.slice(0, 3), 2]);
    const state = createSteppedInteraction({ send: (current, value) => {
        floors.push(current.observations);
        return floors.length === 1 ? first.promise
            : { success: false, outcome: 'unverified', error: 'No new peer evidence', observations: observed };
    } });
    const cached = { ...target('pair', 'same cached status'), observations: original, observationConfirmed: true };
    state.update(cached);
    state.choose(-1);
    state.choose(0);
    first.resolve({ success: true, confirmedValue: -1, observations: observed });
    for (let i = 0; i < 8; i++) await flush();
    assert.deepEqual(floors[1], observed, 'Second write must start beyond the preceding direct readback');
    state.update({ ...cached, projection: 'old peer matches requested zero', observations: observed });
    assert.equal(state.view().failure, 'No new peer evidence', 'Previously read bass cannot confirm the next write');
    state.update({ ...cached, projection: 'fresh zero from both speakers',
        observations: observed.map(row => [...row.slice(0, 3), 3]) });
    assert.equal(state.view().failure, '');
    state.dispose();
});
