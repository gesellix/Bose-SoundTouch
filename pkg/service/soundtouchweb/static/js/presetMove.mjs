import { canonicalLocation } from './contentLocation.mjs';

// Use the same content identity and empty-slot rules as the preset editor.
function presetContent(item) {
    if (!item) return null;
    const source = item.Source ?? item.source ?? '';
    const location = item.Location ?? item.location ?? '';
    const account = item.SourceAccount ?? item.sourceAccount ?? '';
    if (source === 'INVALID_SOURCE' && !location) return null;
    return {
        source,
        sourceAccount: account === source ? '' : account,
        location,
        type: item.Type ?? item.type ?? '',
        itemName: item.ItemName ?? item.itemName ?? '',
        containerArt: item.ContainerArt ?? item.containerArt ?? '',
    };
}

export function currentPresetContent(nowPlaying) {
    const item = nowPlaying?.ContentItem;
    if (['STANDBY', 'INVALID_SOURCE'].includes(nowPlaying?.Source) || !item?.Source || !item?.Location || item.IsPresetable === false) return null;
    const art = nowPlaying.Art;
    return { ...item, ContainerArt: item.ContainerArt || (art?.ArtImageStatus === 'IMAGE_PRESENT' ? art.URL : '') || '' };
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
        // Existing source-only presets can still be compared and preserved.
        // Creating content without a location remains unsupported.
        if (!Number.isInteger(slot) || slot < 1 || slot > 6 || seen.has(slot) || (content && !content.source)) {
            throw new Error('The speaker returned an invalid preset bank.');
        }
        seen.add(slot);
        bank[slot] = content;
    }
    return bank;
}

// A checked read makes three speaker requests. A write may also look up the
// speaker's sources and media-server artwork before sending the mutation.
// Keep the browser budget above the normal server-side request budgets.
const READ_TIMEOUT_MS = 35_000;
const WRITE_TIMEOUT_MS = 45_000;

async function timedRequest(request, timeout) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeout);
    try { return await request(controller.signal); }
    finally { clearTimeout(timer); }
}

function unverified(message, error) {
    return new Error(error?.message ? `${message} Speaker request: ${error.message}` : message);
}

// Avoid overlapping bank edits from different controls in the same player.
const activeEdits = new Set();

async function editBank({ api, deviceId, physicalId, epoch, isCurrent, onStatusReadback }, edit) {
    if (!physicalId || !Number.isSafeInteger(epoch) || epoch < 1 || !isCurrent()) {
        throw new Error('Could not verify the speaker. Reopen the preset editor.');
    }
    const headers = { 'X-AfterTouch-Preset-Target': physicalId, 'X-AfterTouch-Preset-Epoch': String(epoch) };
    if (activeEdits.has(physicalId)) throw new Error('Another preset change is still being checked.');
    async function readBank() {
        if (!isCurrent()) throw new Error('The preset editor is no longer open.');
        const response = await timedRequest(signal => api.devicePresets(deviceId, { headers, signal }), READ_TIMEOUT_MS);
        const data = response?.data;
        if (!isCurrent() || response?.success !== true || data?.info?.device_id !== physicalId || data?.status?.epoch !== epoch) {
            throw new Error('Could not verify the speaker and its presets. Reopen the preset editor.');
        }
        const bank = presetBank(data.status.presets);
        if (Number.isSafeInteger(data.status.revision)) onStatusReadback?.(deviceId, data.status, data.info);
        return bank;
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

    async function write(slot, item) {
        if (!isCurrent()) throw new Error('The preset editor is no longer open.');
        try {
            await timedRequest(signal => item ? api.storePresetContent(deviceId, slot, item, { headers, signal })
                : api.removePreset(deviceId, slot, { headers, signal }), WRITE_TIMEOUT_MS);
            return null;
        } catch (error) {
            if (error?.definitive) throw error;
            // A lost response is ambiguous. Confirm by reading.
            return error;
        }
    }

    activeEdits.add(physicalId);
    try { return await edit({ readBank, waitForBank, write }); }
    finally { activeEdits.delete(physicalId); }
}

const validSlot = slot => Number.isInteger(slot) && slot >= 1 && slot <= 6;

export async function movePreset({ from, to, source, destination, isCurrent, ...target }) {
    if (!validSlot(from) || !validSlot(to) || from === to) {
        throw new Error('Choose a different preset slot.');
    }
    return editBank({ ...target, isCurrent }, async ({ readBank, waitForBank, write }) => {
        const item = presetContent(source);
        const before = await readBank();
        if (!item?.source || !item.location || !samePreset(item, before[from]) ||
            !samePreset(presetContent(destination), before[to])) {
            throw new Error('The source or destination preset changed. Reopen the editor before moving it.');
        }
        const saveError = samePreset(item, before[to], true) ? null : await write(to, item);
        const saved = await waitForBank(bank => samePreset(item, bank[to], true) &&
            before.every((entry, slot) => slot === to || samePreset(entry, bank[slot]) || (slot === from && !bank[slot])));
        if (!saved || !isCurrent()) throw unverified('Move unverified. The player did not clear the original slot. Check both presets.', saveError);
        if (!saved[from]) return;

        let clearError;
        try {
            clearError = await write(from, null);
        } catch (error) {
            throw unverified('The destination was saved, but clearing the original slot was rejected. Check both presets.', error);
        }
        if (!await waitForBank(bank => !bank[from] && saved.every((entry, slot) => slot === from || samePreset(entry, bank[slot])))) {
            throw unverified('The destination was saved, but clearing the original slot is unverified. Check both presets.', clearError);
        }
    });
}

export async function verifyPresetEdit({ slot, content, expected, ...target }) {
    if (!validSlot(slot)) throw new Error('Choose a valid preset slot.');
    const item = presetContent(content);
    if (content !== null && (!item?.source || !item.location)) {
        throw new Error('This content cannot be saved as a preset.');
    }
    return editBank(target, async ({ readBank, waitForBank, write }) => {
        const before = await readBank();
        if (!samePreset(presetContent(expected), before[slot])) {
            throw new Error('The preset changed. Reopen the editor before editing it.');
        }
        if (samePreset(item, before[slot], true)) return;
        const writeError = await write(slot, item);
        const confirmed = await waitForBank(bank => samePreset(item, bank[slot], true) &&
            before.every((entry, index) => index === slot || samePreset(entry, bank[index])));
        if (!confirmed) throw unverified('Preset change unverified. Check the speaker before trying again.', writeError);
    });
}
