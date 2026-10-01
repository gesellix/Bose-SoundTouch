import assert from 'node:assert/strict';
import test from 'node:test';

import {
    directVolumeReadback,
    fenceVolumeReadback,
    maxReadbackActual,
    partialFailureMessage,
} from '../static/js/zoneVolumeResult.mjs';

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

test('accepts only a matching authoritative standalone readback', () => {
    assert.equal(directVolumeReadback({ requested: 41, actual: 41 }, 41), 41);
    assert.equal(directVolumeReadback({ requested: 41, actual: 40 }, 41), null);
    assert.equal(directVolumeReadback({ requested: 39, actual: 40 }, 41), null);
    assert.equal(directVolumeReadback({ requested: 41 }, 41), null);
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
