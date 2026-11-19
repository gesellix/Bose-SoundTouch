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

// Avoid overlapping bank edits from different controls in the same player.
const activeEdits = new Set();

async function editBank({ api, deviceId, physicalId, epoch, isCurrent }, edit) {
    if (!physicalId || !Number.isSafeInteger(epoch) || epoch < 1 || !isCurrent()) {
        throw new Error('Could not verify the speaker. Reopen the preset editor.');
    }
    const headers = { 'X-AfterTouch-Preset-Target': physicalId, 'X-AfterTouch-Preset-Epoch': String(epoch) };
    if (activeEdits.has(physicalId)) throw new Error('Another preset change is still being checked.');
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

    async function write(slot, item) {
        if (!isCurrent()) throw new Error('The preset editor is no longer open.');
        try {
            await timedRequest(signal => item ? api.storePresetContent(deviceId, slot, item, { headers, signal })
                : api.removePreset(deviceId, slot, { headers, signal }));
        } catch (error) {
            if (error?.definitive) throw error;
            // A lost response is ambiguous. Confirm by reading.
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
        if (!samePreset(item, before[to], true)) await write(to, item);
        const saved = await waitForBank(bank => samePreset(item, bank[to], true) &&
            before.every((entry, slot) => slot === to || samePreset(entry, bank[slot]) || (slot === from && !bank[slot])));
        if (!saved || !isCurrent()) throw new Error('Move unverified. The player did not clear the original slot. Check both presets.');
        if (!saved[from]) return;

        await write(from, null);
        if (!await waitForBank(bank => !bank[from] && saved.every((entry, slot) => slot === from || samePreset(entry, bank[slot])))) {
            throw new Error('The destination was saved, but clearing the original slot is unverified. Check both presets.');
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
        await write(slot, item);
        const confirmed = await waitForBank(bank => samePreset(item, bank[slot], true) &&
            before.every((entry, index) => index === slot || samePreset(entry, bank[index])));
        if (!confirmed) throw new Error('Preset change unverified. Check the speaker before trying again.');
    });
}
