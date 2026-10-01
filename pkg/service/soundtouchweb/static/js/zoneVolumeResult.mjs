export function clampVolume(value) {
    return Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0));
}

export function maxReadbackActual(data) {
    const actuals = (data?.members || [])
        .filter(member => Number.isFinite(member.actual))
        .map(member => member.actual);
    return actuals.length > 0 ? clampVolume(Math.max(...actuals)) : null;
}

export function directVolumeReadback(data, requested) {
    if (data?.requested !== requested || data?.actual !== requested) return null;
    return clampVolume(data.actual);
}

export function fenceVolumeReadback(capturedProjection, currentProjection,
    currentVolume, confirmedVolume) {
    const accepted = capturedProjection === currentProjection;
    return {
        accepted,
        volume: accepted ? confirmedVolume : currentVolume,
    };
}

export function partialFailureMessage(data) {
    const failures = (data?.members || []).filter(member => member.error);
    if (failures.length === 0) {
        return data?.partial ? 'Some group members could not be updated.' : '';
    }

    const names = failures.slice(0, 2)
        .map(member => member.name || member.controlId || member.deviceId);
    const remaining = failures.length - names.length;
    const suffix = remaining > 0 ? ` +${remaining}` : '';
    return `${failures.length} ${failures.length === 1 ? 'member' : 'members'} failed: ${names.join(', ')}${suffix}`;
}
