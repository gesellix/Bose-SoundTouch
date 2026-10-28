export function clampVolume(value) {
    return Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0));
}

export function maxReadbackActual(data, projectionKey = null) {
    const current = parseVolumeProjection(projectionKey);
    if (current) {
        const volumes = { ...current.volumes };
        for (const member of data?.members || []) {
            if (member.status !== 'failed' && Number.isFinite(member.actual)) {
                volumes[member.controlId] = member.actual;
            }
        }
        const actuals = Object.values(volumes).filter(Number.isFinite);
        return actuals.length ? clampVolume(Math.max(...actuals)) : null;
    }
    const actuals = (data?.members || [])
        .filter(member => Number.isFinite(member.actual))
        .map(member => member.actual);
    return actuals.length > 0 ? clampVolume(Math.max(...actuals)) : null;
}

export function fenceVolumeReadback(capturedProjection, currentProjection,
    currentVolume, confirmedVolume, data) {
    let accepted = capturedProjection === currentProjection;
    if (!accepted && data) {
        const captured = parseVolumeProjection(capturedProjection);
        const current = parseVolumeProjection(currentProjection);
        // A transport revision alone does not invalidate volume readback. A
        // volume event from this write may also have arrived before its HTTP
        // response. A different physical result must always win over it.
        accepted = Boolean(volumeProjectionAtLeast(captured, current) && (
            JSON.stringify(captured.volumes) === JSON.stringify(current.volumes) ||
            volumeResultCompatible(data, captured.volumes, current.volumes)));
    }
    return {
        accepted,
        volume: accepted ? confirmedVolume : currentVolume,
    };
}

export function volumeProjectionKey(epoch, revision, volumes) {
    return JSON.stringify({ epoch, revision, volumes: Object.fromEntries(
        Object.entries(volumes).sort(([left], [right]) => left.localeCompare(right))) });
}

function parseVolumeProjection(key) {
    try {
        const projection = JSON.parse(key);
        return projection?.volumes ? projection : null;
    } catch {
        return null;
    }
}

function volumeResultMatches(data, volumes, field) {
    const members = data?.members || [];
    return members.length > 0 && members.length === Object.keys(volumes || {}).length &&
        members.every(member => Number.isFinite(member[field]) &&
            volumes[member.controlId] === member[field]);
}

function volumeResultCompatible(data, before, current) {
    const members = data?.members || [];
    return members.length > 0 && members.length === Object.keys(current || {}).length &&
        members.length === Object.keys(before || {}).length && members.every(member => {
            const volume = current[member.controlId];
            return Number.isFinite(volume) && (volume === before[member.controlId] ||
                volume === member.actual || (member.status !== 'failed' && volume === member.target));
        });
}

// Only the same requested member targets can resolve an unverified final.
// A definitive rejection remains visible even if those volumes later happen
// to be observed. No new network request is needed for this reconciliation.
function projectionRevisions(revision) {
    if (typeof revision === 'string') {
        try { revision = JSON.parse(revision); } catch { return []; }
    }
    if (revision && typeof revision === 'object' && !Array.isArray(revision)) {
        return Object.keys(revision).sort().map(id => revision[id]);
    }
    return Array.isArray(revision) ? revision : [revision];
}

function volumeProjectionAtLeast(captured, current) {
    if (!captured || !current || captured.epoch !== current.epoch) return false;
    const before = projectionRevisions(captured.revision);
    const after = projectionRevisions(current.revision);
    return before.length > 0 && before.length === after.length &&
        before.every((revision, index) => Number.isFinite(revision) &&
            Number.isFinite(after[index]) && after[index] >= revision);
}

function newerVolumeProjection(captured, current, data) {
    if (!volumeProjectionAtLeast(captured, current) ||
        JSON.stringify(captured.volumes) === JSON.stringify(current.volumes)) return false;
    const unverified = data.members.filter(member => member.status === 'unverified');
    if (unverified.some(member => captured.volumes[member.controlId] ===
        current.volumes[member.controlId])) return false;
    if (typeof captured.epoch === 'string') {
        let epochs;
        try { epochs = JSON.parse(captured.epoch); } catch { epochs = null; }
        if (epochs && typeof epochs === 'object' && unverified.some(member =>
            epochs[member.controlId] === null || epochs[member.controlId] === undefined)) return false;
    }
    const before = projectionRevisions(captured.revision);
    const after = projectionRevisions(current.revision);
    if (typeof captured.revision === 'string' && typeof current.revision === 'string') {
        const beforeMembers = JSON.parse(captured.revision);
        const afterMembers = JSON.parse(current.revision);
        if (beforeMembers && typeof beforeMembers === 'object' && !Array.isArray(beforeMembers)) {
            return Object.keys(current.volumes).every(id =>
                captured.volumes[id] === current.volumes[id] ||
                (Number.isFinite(beforeMembers[id]) && afterMembers[id] > beforeMembers[id]));
        }
    }
    return before.some((revision, index) => after[index] > revision);
}

export function reconcileVolumeResult(data, projectionKey, capturedProjection) {
    const projection = parseVolumeProjection(projectionKey);
    const captured = parseVolumeProjection(capturedProjection);
    if (!projection || !data?.members?.length) return data;
    const canConfirm = !data.members.some(member =>
        member.status === 'failed' || (!member.status && member.error)) &&
        volumeResultMatches(data, projection.volumes, 'target') &&
        (!data.members.some(member => member.status === 'unverified') ||
            newerVolumeProjection(captured, projection, data));
    const members = data.members.map(member => {
        // One member may already have reached its target while another is
        // still unverified. Preserve that newer actual without clearing the
        // partial result or substituting the older HTTP actual over it.
        if (!canConfirm && member.status === 'unverified' &&
            projection.volumes[member.controlId] === member.target &&
            newerVolumeProjection(captured, projection, { members: [member] })) {
            return { ...member, actual: projection.volumes[member.controlId] };
        }
        if (canConfirm || (member.status === 'confirmed' &&
            projection.volumes[member.controlId] === member.target)) {
            return { ...member, actual: projection.volumes[member.controlId],
                status: 'confirmed', error: '' };
        }
        return member;
    });
    if (members.every((member, index) => member === data.members[index])) return data;
    return { ...data, partial: canConfirm ? false : data.partial, members };
}

export function partialFailureMessage(data) {
    const members = data?.members || [];
    const summaries = ['failed', 'unverified'].map(status => {
        const affected = members.filter(member =>
            member.status === status || (status === 'failed' && !member.status && member.error));
        if (affected.length === 0) return '';
        const names = affected.slice(0, 2)
            .map(member => member.name || member.controlId || member.deviceId);
        const remaining = affected.length - names.length;
        const suffix = remaining > 0 ? ` +${remaining}` : '';
        return `${affected.length} ${affected.length === 1 ? 'member' : 'members'} ${status}: ${names.join(', ')}${suffix}`;
    }).filter(Boolean);
    return summaries.join('; ') || (data?.partial ? 'Some member updates are unverified.' : '');
}
