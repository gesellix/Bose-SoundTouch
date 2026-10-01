import { clampVolume } from './zoneVolumeResult.mjs';

export function zoneMemberVolumes(zone) {
    const volumes = {};

    for (const member of zone?.members || []) {
        const controlId = member?.controlId || member?.ip;
        if (!controlId || !Number.isFinite(member?.actualVolume)) continue;
        volumes[controlId] = clampVolume(Math.round(member.actualVolume));
    }

    return volumes;
}

export function maxZoneVolume(volumes) {
    const values = Object.values(volumes || {}).filter(Number.isFinite);
    return values.length > 0 ? Math.max(...values) : 0;
}

export function previewZoneVolume(startingVolumes, startingLevel, requested) {
    const entries = Object.entries(startingVolumes || {})
        .filter(([, volume]) => Number.isFinite(volume));
    if (entries.length === 0) return {};

    const delta = clampVolume(requested) - clampVolume(startingLevel);
    return Object.fromEntries(entries.map(([controlId, volume]) => [
        controlId,
        clampVolume(volume + delta),
    ]));
}

export function mergeZoneVolumeReadback(currentVolumes, data) {
    const volumes = { ...(currentVolumes || {}) };

    for (const member of data?.members || []) {
        if (!member?.controlId || !Number.isFinite(member.actual)) continue;
        volumes[member.controlId] = clampVolume(member.actual);
    }

    return volumes;
}

export function sameZoneMemberVolumes(left, right) {
    const leftEntries = Object.entries(left || {}).sort(([a], [b]) => a.localeCompare(b));
    const rightEntries = Object.entries(right || {}).sort(([a], [b]) => a.localeCompare(b));

    return leftEntries.length === rightEntries.length && leftEntries.every(
        ([controlId, volume], index) =>
            rightEntries[index][0] === controlId && rightEntries[index][1] === volume,
    );
}
