import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

// Use the existing app merge functions without importing browser-only modules.
const source = await readFile(new URL('../static/js/app.js', import.meta.url), 'utf8');
const start = source.indexOf('function statusRevision(');
const end = source.indexOf('export function DeviceDetail(');
const { mergeDevicesSnapshot, mergeStatusUpdate } = new Function(
    `${source.slice(start, end).replaceAll('export function', 'function')}\nreturn { mergeDevicesSnapshot, mergeStatusUpdate };`,
)();
const member = (id, actualVolume) => ({ controlId: id, hwId: id.toUpperCase(),
    deviceIds: [id.toUpperCase()], actualVolume, available: true });
const device = (id, volume, revision = 1, epoch = 1) => ({ info: { device_id: id.toUpperCase() },
    status: { epoch, revision, volume: { ActualVolume: volume } } });
function inventory() {
    const master = device('master', 20, 5);
    master.zone = { masterControlId: 'master', masterDeviceId: 'MASTER', volume: 20,
        members: [member('master', 20), member('child', 10)] };
    return { master, child: device('child', 10) };
}

test('member-only accepted event refreshes group volume without advancing master revision', () => {
    const previous = inventory();
    const merged = mergeStatusUpdate(previous, 'child', { epoch: 1, revision: 2, volume: { ActualVolume: 30 } });
    assert.equal(merged.master.status, previous.master.status);
    assert.equal(merged.master.zone.members[1].actualVolume, 30);
    assert.equal(merged.master.zone.volume, 30);
    assert.equal(previous.master.zone.members[1].actualVolume, 10);
});

test('equal master snapshot adopts accepted member volume while retaining master topology', () => {
    const previous = inventory();
    const snapshot = inventory();
    snapshot.child = device('child', 25, 2);
    snapshot.master.zone.members.push(member('intruder', 50));
    const merged = mergeDevicesSnapshot(previous, snapshot);
    assert.equal(merged.master.status, previous.master.status);
    assert.deepEqual(merged.master.zone.members.map(member => member.controlId), ['master', 'child']);
    assert.equal(merged.master.zone.members[1].actualVolume, 25);
    assert.equal(merged.master.zone.volume, 25);
});

test('stale member delta and mismatched hardware cannot supply group volume', () => {
    const previous = inventory();
    const old = mergeStatusUpdate(previous, 'child', { epoch: 1, revision: 0, volume: { ActualVolume: 99 } });
    assert.equal(old, previous);
    const snapshot = inventory();
    snapshot.child = { ...device('child', 99, 2), info: { device_id: 'REPLACEMENT' } };
    const merged = mergeDevicesSnapshot(previous, snapshot);
    assert.equal(merged.master.zone.members[1].actualVolume, 10);
});

test('stereo member volume refresh requires the same logical pair and master', () => {
    const previous = inventory();
    const pair = { id: 'pair', masterDeviceId: 'CHILD', members: [
        { deviceId: 'CHILD', role: 'LEFT' }, { deviceId: 'RIGHT', role: 'RIGHT' } ] };
    previous.child.stereoPair = pair;
    previous.child.status.group = { ID: pair.id, MasterDeviceID: pair.masterDeviceId,
        Roles: { Roles: pair.members.map(member => ({ DeviceID: member.deviceId, Role: member.role })) } };
    previous.master.zone.members[1] = { ...member('child', 10),
        deviceIds: ['CHILD', 'RIGHT'], stereoPair: pair };
    const snapshot = structuredClone(previous);
    snapshot.child.status = { ...snapshot.child.status, revision: 2, volume: { ActualVolume: 30 } };
    snapshot.child.stereoPair.members.reverse();
    assert.equal(mergeDevicesSnapshot(previous, snapshot).master.zone.members[1].actualVolume, 30);
    snapshot.child.status.group = {};
    assert.equal(mergeDevicesSnapshot(previous, snapshot).master.zone.members[1].actualVolume, 10);
    snapshot.child.stereoPair.id = 'replacement';
    assert.equal(mergeDevicesSnapshot(previous, snapshot).master.zone.members[1].actualVolume, 10);
    delete snapshot.child;
    assert.equal(mergeDevicesSnapshot(previous, snapshot).master.zone.members[1].actualVolume, 10);
});

test('status-only connection replacement waits for accepted identity inventory', () => {
    const previous = inventory();
    const replaced = mergeStatusUpdate(previous, 'child', { epoch: 2, revision: 1, volume: { ActualVolume: 80 } });
    assert.equal(replaced.master.zone.members[1].actualVolume, 10);
    const delta = mergeStatusUpdate(replaced, 'child', { epoch: 2, revision: 2, volume: { ActualVolume: 81 } });
    assert.equal(delta.master.zone.members[1].actualVolume, 10);
    const snapshot = inventory();
    snapshot.child = device('child', 30, 3, 2);
    assert.equal(mergeDevicesSnapshot(delta, snapshot).master.zone.members[1].actualVolume, 30);
    snapshot.child.info.device_id = 'REPLACEMENT';
    assert.equal(mergeDevicesSnapshot(delta, snapshot).master.zone.members[1].actualVolume, 10);
});
