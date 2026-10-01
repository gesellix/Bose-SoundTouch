import assert from 'node:assert/strict';
import test from 'node:test';

import {
    mergeZoneVolumeReadback,
    previewZoneVolume,
    sameZoneMemberVolumes,
    zoneMemberVolumes,
} from '../static/js/zoneVolumePreview.mjs';

test('projects one shared delta across logical members without splitting stereo pairs', () => {
    assert.deepEqual(previewZoneVolume({ kitchen: 20, stereo: 35 }, 35, 42), {
        kitchen: 27,
        stereo: 42,
    });
});

test('extracts authoritative logical volumes and merges successful readbacks', () => {
    const projected = zoneMemberVolumes({
        members: [
            { controlId: 'kitchen', actualVolume: 21 },
            { controlId: 'stereo', actualVolume: 34, physicalMembers: [
                { role: 'left' },
                { role: 'right' },
            ] },
        ],
    });

    assert.deepEqual(projected, { kitchen: 21, stereo: 34 });
    assert.deepEqual(mergeZoneVolumeReadback(projected, {
        members: [
            { controlId: 'kitchen', actual: 25 },
            { controlId: 'stereo', error: 'readback failed' },
        ],
    }), { kitchen: 25, stereo: 34 });
});

test('requires the complete authoritative map before dropping an optimistic overlay', () => {
    assert.equal(sameZoneMemberVolumes({ a: 10, b: 20 }, { b: 20, a: 10 }), true);
    assert.equal(sameZoneMemberVolumes({ a: 10 }, { a: 10, b: 20 }), false);
    assert.equal(sameZoneMemberVolumes({ a: 10 }, { a: 11 }), false);
});
