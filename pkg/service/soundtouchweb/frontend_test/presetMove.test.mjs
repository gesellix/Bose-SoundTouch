import assert from 'node:assert/strict';
import test from 'node:test';
import { movePreset, verifyPresetEdit } from '../static/js/presetMove.mjs';

const source = { Source: 'TUNEIN', Location: 's12345', ItemName: 'WDR 2' };
const preset = (ID, ContentItem = source) => ({ ID, ContentItem });

function setup({ afterStore, afterDelete, before = [preset(1)] }) {
    let bank = before;
    const calls = [];
    const options = {
        deviceId: 'speaker', physicalId: 'DEVICE', epoch: 1, from: 1, to: 3, source,
        isCurrent: () => true,
        api: {
            async devicePresets(_, { headers }) {
                assert.equal(headers['X-AfterTouch-Preset-Target'], 'DEVICE');
                assert.equal(headers['X-AfterTouch-Preset-Epoch'], '1');
                calls.push('read');
                return { success: true, data: { info: { device_id: 'DEVICE' }, status: { epoch: 1, presets: { Preset: bank } } } };
            },
            async storePresetContent(_, slot) {
                calls.push(`write ${slot}`);
                if (afterStore) bank = afterStore;
                return { success: true };
            },
            async removePreset(_, slot) {
                calls.push(`clear ${slot}`);
                if (afterDelete) bank = afterDelete;
                return { success: true };
            },
        },
    };
    return { options, calls };
}

test('an acknowledged store without destination evidence never deletes the original preset', async () => {
    const { options, calls } = setup({});
    await assert.rejects(movePreset(options), /did not clear the original slot/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'read', 'read']);
});

test('a contradictory destination readback cannot authorize deletion', async () => {
    const { options, calls } = setup({ afterStore: [preset(1), preset(3, null), preset(3)] });
    await assert.rejects(movePreset(options), /did not clear the original slot/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'read', 'read']);
});

test('a destination that changed since opening requires a new confirmation', async () => {
    const { options, calls } = setup({ before: [preset(1), preset(3, { ...source, Location: 's4321' })] });
    await assert.rejects(movePreset(options), /source or destination preset changed/);
    assert.deepEqual(calls, ['read']);
});

test('a firmware move needs no second write when the original slot is already empty', async () => {
    const { options, calls } = setup({ afterStore: [preset(0, null), preset(1, null), preset(3)] });
    await movePreset(options);
    assert.deepEqual(calls, ['read', 'write 3', 'read']);
});

test('an acknowledged delete without source-clear evidence leaves the move unverified', async () => {
    const { options, calls } = setup({ afterStore: [preset(1), preset(3)] });
    await assert.rejects(movePreset(options), /clearing the original slot is unverified/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'clear 1', 'read', 'read', 'read']);
});

test('an acknowledged save or clear without a changed preset is not confirmed', async () => {
    for (const { slot, content, expected, write } of [
        { slot: 3, content: source, expected: null, write: 'write 3' },
        { slot: 1, content: null, expected: source, write: 'clear 1' },
    ]) {
        const { options, calls } = setup({});
        await assert.rejects(verifyPresetEdit({ ...options, slot, content, expected }), /Preset change unverified/);
        assert.deepEqual(calls, ['read', write, 'read', 'read', 'read']);
    }
});

test('a lost write response can confirm once the speaker reports the selected content', async () => {
    const { options, calls } = setup({ afterStore: [preset(1), preset(3)] });
    const store = options.api.storePresetContent;
    options.api.storePresetContent = async (...args) => { await store(...args); throw new Error('response lost'); };
    await verifyPresetEdit({ ...options, slot: 3, content: source, expected: null });
    assert.deepEqual(calls, ['read', 'write 3', 'read']);
});

test('losing a differently named preset for the same station prevents confirmation', async () => {
    const another = { ...source, ItemName: 'WDR 2 alternate' };
    const { options, calls } = setup({ before: [preset(1, another)], afterStore: [preset(3)] });
    await assert.rejects(verifyPresetEdit({ ...options, slot: 3, content: source, expected: null }), /Preset change unverified/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'read', 'read']);
});
