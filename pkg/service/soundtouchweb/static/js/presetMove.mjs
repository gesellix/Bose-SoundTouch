import { canonicalLocation } from './contentLocation.mjs';

// Use the same content identity and empty-slot rules as the preset editor.
function presetContent(item) {
    if (!item || (item.Source === 'INVALID_SOURCE' && !item.Location)) return null;
    return {
        source: item.Source || '',
        sourceAccount: item.SourceAccount === item.Source ? '' : item.SourceAccount || '',
        location: item.Location || '',
        type: item.Type || '',
        itemName: item.ItemName || '',
        containerArt: item.ContainerArt || '',
    };
}

function samePreset(expected, actual, allowNormalization = false) {
    if (!expected || !actual) return expected === actual;
    return expected.source === actual.source && expected.sourceAccount === actual.sourceAccount &&
        canonicalLocation(expected.source, expected.location) === canonicalLocation(actual.source, actual.location) &&
        expected.itemName === actual.itemName &&
        (expected.type === actual.type || (allowNormalization && !expected.type)) &&
        (expected.containerArt === actual.containerArt || (allowNormalization && !expected.containerArt));
}

function presetBank(presets) {
    const entries = presets?.Preset ?? [];
    if (!presets || !Array.isArray(entries)) throw new Error('The speaker did not return its preset bank.');
    const bank = Array(7).fill(null); // Slots are numbered 1…6.
    const seen = new Set();
    for (const entry of entries) {
        const content = presetContent(entry?.ContentItem);
        const slot = Number(entry?.ID);
        if (slot === 0 && !content) continue;
        if (!Number.isInteger(slot) || slot < 1 || slot > 6 || seen.has(slot) || (content && (!content.source || !content.location))) {
            throw new Error('The speaker returned an invalid preset bank.');
        }
        seen.add(slot);
        bank[slot] = content;
    }
    return bank;
}

async function timedRequest(request) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 4000);
    try { return await request(controller.signal); }
    finally { clearTimeout(timer); }
}

export async function movePreset({ api, deviceId, physicalId, epoch, from, to, source, destination, isCurrent }) {
    if (![from, to].every(slot => Number.isInteger(slot) && slot >= 1 && slot <= 6) || from === to) {
        throw new Error('Choose a different preset slot.');
    }
    if (!physicalId || !Number.isSafeInteger(epoch) || epoch < 1 || !isCurrent()) {
        throw new Error('Could not verify the speaker. Reopen the preset editor.');
    }
    const headers = { 'X-AfterTouch-Preset-Target': physicalId, 'X-AfterTouch-Preset-Epoch': String(epoch) };
    async function readBank() {
        if (!isCurrent()) throw new Error('The preset editor is no longer open.');
        const response = await timedRequest(signal => api.devicePresets(deviceId, { headers, signal }));
        const data = response?.data;
        if (!isCurrent() || response?.success !== true || data?.info?.device_id !== physicalId || data?.status?.epoch !== epoch) {
            throw new Error('Could not verify the speaker and its presets. Reopen the preset editor.');
        }
        return presetBank(data.status.presets);
    }

    async function waitForBank(matches) {
        for (const delay of [0, 150, 500]) {
            if (delay) await new Promise(resolve => setTimeout(resolve, delay));
            if (!isCurrent()) return null;
            try {
                const bank = await readBank();
                if (matches(bank)) return bank;
            } catch (_) { /* Retry only the read, never the write. */ }
        }
        return null;
    }

    const item = presetContent(source);
    const before = await readBank();
    if (!item?.source || !item.location || !samePreset(item, before[from]) ||
        !samePreset(presetContent(destination), before[to])) {
        throw new Error('The source or destination preset changed. Reopen the editor before moving it.');
    }
    if (!isCurrent()) return;
    if (!samePreset(item, before[to], true)) {
        try { await timedRequest(signal => api.storePresetContent(deviceId, to, item, { headers, signal })); }
        catch (_) { /* The write may already have happened. Confirm by reading. */ }
    }
    const saved = await waitForBank(bank => samePreset(item, bank[to], true) &&
        before.every((entry, slot) => slot === to || samePreset(entry, bank[slot]) || (slot === from && !bank[slot])));
    if (!saved || !isCurrent()) throw new Error('Move unverified. The player did not clear the original slot. Check both presets.');
    if (!saved[from]) return;

    try { await timedRequest(signal => api.removePreset(deviceId, from, { headers, signal })); }
    catch (_) { /* Confirm the resulting bank even if the reply was lost. */ }
    if (!await waitForBank(bank => !bank[from] && saved.every((entry, slot) => slot === from || samePreset(entry, bank[slot])))) {
        throw new Error('The destination was saved, but clearing the original slot is unverified. Check both presets.');
    }
}
