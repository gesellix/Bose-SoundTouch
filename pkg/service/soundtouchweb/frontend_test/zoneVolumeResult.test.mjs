import assert from 'node:assert/strict';
import test from 'node:test';

import {
    fenceVolumeReadback,
    maxReadbackActual,
    partialFailureMessage,
    reconcileVolumeResult,
    volumeProjectionKey,
} from '../static/js/zoneVolumeResult.mjs';

test('partial group readback retains a failed loudest member in the displayed maximum', () => {
    const projection = volumeProjectionKey(1, 5, { master: 90, member: 20 });
    const data = { members: [
        { controlId: 'master', target: 100, status: 'failed' },
        { controlId: 'member', target: 30, actual: 30, status: 'confirmed' },
    ] };
    assert.equal(maxReadbackActual(data, projection), 90);
    assert.match(partialFailureMessage(data), /1 member failed/);
    data.members[0].actual = 0;
    assert.equal(maxReadbackActual(data, projection), 90);
});

test('partial progress preserves a newer member target without clearing the remaining warning', () => {
    const epochs = JSON.stringify({ master: 1, child: 1 });
    const captured = volumeProjectionKey(epochs, JSON.stringify({ master: 1, child: 1 }), { master: 20, child: 0 });
    const current = volumeProjectionKey(epochs, JSON.stringify({ master: 2, child: 1 }), { master: 40, child: 0 });
    const data = { partial: true, members: [
        { controlId: 'master', target: 40, actual: 35, status: 'unverified' },
        { controlId: 'child', target: 20, actual: 15, status: 'unverified' },
    ] };
    const result = reconcileVolumeResult(data, current, captured);
    assert.equal(maxReadbackActual(result, current), 40);
    assert.equal(result.members[0].actual, 40);
    assert.equal(result.members[0].status, 'unverified');
    assert.equal(result.partial, true);
    assert.match(partialFailureMessage(result), /2 members unverified/);
    const old = volumeProjectionKey(epochs, JSON.stringify({ master: 0, child: 1 }), { master: 40, child: 0 });
    assert.equal(reconcileVolumeResult(data, old, captured), data);
});

test('a newer projection wins over an older successful HTTP readback', () => {
    assert.deepEqual(fenceVolumeReadback('revision-1', 'revision-2', 20, 35), {
        accepted: false,
        volume: 20,
    });
    assert.deepEqual(fenceVolumeReadback('revision-2', 'revision-2', 20, 35), {
        accepted: true,
        volume: 35,
    });
});

test('keeps successful readback and names partial failures concisely', () => {
    const data = {
        partial: true,
        members: [
            { name: 'Kitchen', actual: 50 },
            { name: 'Living room', actual: 80, error: 'set volume: timeout' },
        ],
    };

    assert.equal(maxReadbackActual(data), 80);
    assert.equal(partialFailureMessage(data), '1 member failed: Living room');
});

test('caps long failure lists at two names while retaining the count', () => {
    assert.equal(partialFailureMessage({
        members: [
            { name: 'Kitchen', error: 'failed' },
            { name: 'Hall', error: 'failed' },
            { name: 'Office', error: 'failed' },
        ],
    }), '3 members failed: Kitchen, Hall +1');
});

test('unverified confirmation is not labeled as a failed write', () => {
    assert.equal(partialFailureMessage({
        partial: true,
        members: [
            { name: 'Kitchen', status: 'confirmed', actual: 25 },
            { name: 'Hall', status: 'unverified', error: 'Readback unavailable' },
            { name: 'Office', status: 'failed', error: 'Rejected' },
        ],
    }), '1 member failed: Office; 1 member unverified: Hall');
});

test('readback accepts its own speaker confirmation and transport-only revisions', () => {
    const captured = volumeProjectionKey('epoch', 1, { master: 20, member: 10 });
    const data = { members: [
        { controlId: 'master', actual: 30, target: 30 },
        { controlId: 'member', actual: 20, target: 20 },
    ] };
    for (const volumes of [{ master: 20, member: 10 }, { master: 30, member: 20 },
        { master: 30, member: 10 }]) {
        assert.equal(fenceVolumeReadback(captured,
            volumeProjectionKey('epoch', 2, volumes), Math.max(...Object.values(volumes)), 30,
            data).accepted, true);
    }
    assert.deepEqual(fenceVolumeReadback(captured,
        volumeProjectionKey('epoch', 3, { master: 25, member: 15 }), 25, 30, data), {
        accepted: false, volume: 25,
    });
    assert.equal(fenceVolumeReadback(captured,
        volumeProjectionKey('next-epoch', 1, { master: 30, member: 20 }), 30, 30,
        data).accepted, false);
});

test('unverified final needs a later projection confirming every original member target', () => {
    const data = { partial: true, members: [
        { controlId: 'master', status: 'confirmed', actual: 20, target: 30 },
        { controlId: 'member', status: 'unverified', actual: 10, target: 20, error: 'Readback unavailable' },
    ] };
    const published = volumeProjectionKey('epoch', 5, { master: 20, member: 10 });
    for (const current of [
        volumeProjectionKey('epoch', 6, { master: 20, member: 10 }),
        volumeProjectionKey('epoch', 6, { master: 30, member: 10 }),
        volumeProjectionKey('epoch', 6, { master: 30 }),
        volumeProjectionKey('epoch', 5, { master: 30, member: 20 }),
        volumeProjectionKey('epoch', 4, { master: 30, member: 20 }),
        volumeProjectionKey('next-epoch', 6, { master: 30, member: 20 }),
    ]) {
        assert.equal(partialFailureMessage(reconcileVolumeResult(data, current, published)),
            partialFailureMessage(data));
    }
    const confirmed = reconcileVolumeResult(data,
        volumeProjectionKey('epoch', 6, { master: 30, member: 20 }), published);
    assert.equal(partialFailureMessage(confirmed), '');
    assert.equal(confirmed.partial, false);
    assert.deepEqual(confirmed.members.map(member => [member.status, member.actual]),
        [['confirmed', 30], ['confirmed', 20]]);
});

test('definitive failure persists even when later member targets match', () => {
    const data = { partial: true, members: [
        { controlId: 'member', status: 'failed', actual: 10, target: 20, error: 'Rejected' },
    ] };
    assert.equal(reconcileVolumeResult(data,
        volumeProjectionKey('epoch', 2, { member: 20 }),
        volumeProjectionKey('epoch', 1, { member: 10 })), data);
    assert.equal(partialFailureMessage(data), '1 member failed: member');
});

test('confirmed response cannot pull a matching newer target back to its old ramping actual', () => {
    const data = { members: [{ controlId: 'member', status: 'confirmed', target: 30, actual: 20 }] };
    const current = volumeProjectionKey('epoch', 2, { member: 30 });
    const confirmed = reconcileVolumeResult(data, current);
    assert.equal(maxReadbackActual(confirmed), 30);
});

test('group recovery needs each unverified member own changed volume and normal revision', () => {
    const key = (revisions, volumes, epochs = { master: 1, member: 1 }) =>
        volumeProjectionKey(JSON.stringify(epochs), JSON.stringify(revisions), volumes);
    const data = { partial: true, members: [
        { controlId: 'master', status: 'confirmed', target: 30, actual: 20 },
        { controlId: 'member', status: 'unverified', target: 20, actual: 10 },
    ] };
    const captured = key({ master: 5, member: 5 }, { master: 20, member: 10 });
    assert.equal(partialFailureMessage(reconcileVolumeResult(data,
        key({ master: 5, member: 6 }, { master: 30, member: 20 }), captured)),
        partialFailureMessage(data)); // Master volume changed without its revision.
    assert.equal(partialFailureMessage(reconcileVolumeResult(data,
        key({ master: 6, member: 5 }, { master: 30, member: 20 }), captured)),
        partialFailureMessage(data));
    assert.equal(partialFailureMessage(reconcileVolumeResult(data,
        key({ master: 6, member: 6 }, { master: 30, member: 20 }, { master: 1, member: 2 }),
        captured)), partialFailureMessage(data));
    assert.equal(partialFailureMessage(reconcileVolumeResult(data,
        key({ master: 6, member: 6 }, { master: 30, member: 20 }), captured)), '');
    const alreadyAtTarget = key({ master: 5, member: 5 }, { master: 20, member: 20 });
    assert.equal(partialFailureMessage(reconcileVolumeResult(data,
        key({ master: 6, member: 6 }, { master: 30, member: 20 }), alreadyAtTarget)),
        partialFailureMessage(data));
});

test('lower member revisions cannot authorize readback even for equivalent volumes', () => {
    const captured = volumeProjectionKey('epoch', 5, { member: 20 });
    assert.equal(fenceVolumeReadback(captured,
        volumeProjectionKey('epoch', 4, { member: 20 }), 20, 30,
        { members: [{ controlId: 'member', actual: 30, target: 30 }] }).accepted, false);
});
