import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

// Exercise the existing app merge functions without loading browser imports.
const source = await readFile(new URL('../static/js/app.js', import.meta.url), 'utf8');
const start = source.indexOf('function statusRevision(');
const end = source.indexOf('export function DeviceDetail(');
const { mergeDevicesSnapshot, mergeStatusUpdate } = new Function(
    `${source.slice(start, end).replaceAll('export function', 'function')}\nreturn { mergeDevicesSnapshot, mergeStatusUpdate };`,
)();

const status = (revision, epoch = 1) => ({ epoch, revision, bass: { ActualBass: -revision } });
const device = (masterRevision, childRevision, pairId = 'pair', childEpoch = 1) => ({
    info: { device_id: 'master' }, status: status(masterRevision),
    stereoPair: { id: pairId, masterDeviceId: 'master', members: [{ deviceId: 'master', role: 'RIGHT' }, { deviceId: 'child', role: 'LEFT' }] },
    settingsTargets: [{ controlId: 'child.local', physicalId: 'child', role: 'LEFT', status: status(childRevision, childEpoch) }],
});

test('equal master revision accepts fresh child status without accepting stale topology', () => {
    const current = device(5, 2);
    const incoming = device(5, 3);
    const merged = mergeDevicesSnapshot({ master: current }, { master: incoming }).master;
    assert.equal(merged.status, current.status);
    assert.equal(merged.stereoPair, current.stereoPair);
    assert.equal(merged.settingsTargets[0].status, incoming.settingsTargets[0].status);
});

test('fresh master retains fresher matching child and accepts a new child epoch with lower revision', () => {
    const current = device(5, 7);
    const incoming = device(6, 3);
    const merged = mergeDevicesSnapshot({ master: current }, { master: incoming }).master;
    assert.equal(merged.status, incoming.status);
    assert.equal(merged.settingsTargets[0].status, current.settingsTargets[0].status);
    const reconnect = device(6, 1, 'pair', 2);
    assert.equal(mergeDevicesSnapshot({ master: merged }, { master: reconnect }).master.settingsTargets[0].status.epoch, 2);
});

test('accepted membership changes never retain children from a prior pair or registry target', () => {
    const current = device(5, 9, 'old-pair');
    const incoming = device(6, 1, 'new-pair');
    assert.equal(mergeDevicesSnapshot({ master: current }, { master: incoming }).master.settingsTargets[0].status.revision, 1);
    const stale = device(5, 10, 'old-pair');
    assert.equal(mergeDevicesSnapshot({ master: incoming }, { master: stale }).master.settingsTargets[0].status.revision, 1);
    const replaced = device(7, 2, 'new-pair');
    replaced.settingsTargets[0].controlId = 'replacement.local';
    assert.equal(mergeDevicesSnapshot({ master: incoming }, { master: replaced }).master.settingsTargets[0].status.revision, 2);
});

test('hidden status delta needs an existing matching hardware identity and never creates a target', () => {
    const current = { master: device(5, 2) };
    assert.equal(mergeStatusUpdate(current, 'child.local', status(3)), current);
    const update = { ...status(3), bass: { DeviceID: 'child', ActualBass: -3 } };
    assert.equal(mergeStatusUpdate(current, 'child.local', update).master.settingsTargets[0].status, update);
    const wrong = { ...update, bass: { DeviceID: 'replacement', ActualBass: -3 } };
    assert.equal(mergeStatusUpdate(current, 'child.local', wrong).master.settingsTargets[0].status, current.master.settingsTargets[0].status);
    assert.equal(mergeStatusUpdate(current, 'unknown.local', update).master.settingsTargets.length, 1);
});
