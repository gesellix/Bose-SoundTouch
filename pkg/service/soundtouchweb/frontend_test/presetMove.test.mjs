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

test('a definite clear rejection identifies the saved destination and keeps the speaker error', async () => {
    const { options, calls } = setup({ afterStore: [preset(1), preset(3)] });
    const remove = options.api.removePreset;
    options.api.removePreset = async (...args) => {
        await remove(...args);
        throw Object.assign(new Error('Target changed'), { definitive: true });
    };
    await assert.rejects(movePreset(options), /destination was saved.*Target changed/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'clear 1']);
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

test('an ambiguous speaker error remains visible if the mutation cannot be confirmed', async () => {
    const { options, calls } = setup({});
    const store = options.api.storePresetContent;
    options.api.storePresetContent = async (...args) => {
        await store(...args);
        throw new Error('Speaker refused the item');
    };
    await assert.rejects(verifyPresetEdit({ ...options, slot: 3, content: source, expected: null }),
        /Preset change unverified.*Speaker refused the item/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'read', 'read']);
});

test('a slow checked write has time for best-effort artwork before readback', async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const { options, calls } = setup({ afterStore: [preset(1), preset(3)] });
    const store = options.api.storePresetContent;
    options.api.storePresetContent = async (_, slot, item, { signal }) => {
        t.mock.timers.tick(4_500);
        assert.equal(signal.aborted, false);
        return store(_, slot, item);
    };
    await verifyPresetEdit({ ...options, slot: 3, content: source, expected: null });
    assert.deepEqual(calls, ['read', 'write 3', 'read']);
});

test('a source-only existing preset can be preserved while editing another slot', async () => {
    const input = preset(2, { Source: 'AUX', ItemName: 'Aux input' });
    const { options, calls } = setup({ before: [input], afterStore: [input, preset(3)] });
    await verifyPresetEdit({ ...options, slot: 3, content: source, expected: null });
    assert.deepEqual(calls, ['read', 'write 3', 'read']);
});

test('unexpected removal of a source-only preset is still unverified', async () => {
    const input = preset(2, { Source: 'AUX', ItemName: 'Aux input' });
    const { options, calls } = setup({ before: [input], afterStore: [preset(3)] });
    await assert.rejects(verifyPresetEdit({ ...options, slot: 3, content: source, expected: null }),
        /Preset change unverified/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'read', 'read']);
});

test('losing a differently named preset for the same station prevents confirmation', async () => {
    const another = { ...source, ItemName: 'WDR 2 alternate' };
    const { options, calls } = setup({ before: [preset(1, another)], afterStore: [preset(3)] });
    await assert.rejects(verifyPresetEdit({ ...options, slot: 3, content: source, expected: null }), /Preset change unverified/);
    assert.deepEqual(calls, ['read', 'write 3', 'read', 'read', 'read']);
});
