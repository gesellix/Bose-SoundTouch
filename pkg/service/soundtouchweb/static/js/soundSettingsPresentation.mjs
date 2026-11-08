import { observationsAdvanced } from './steppedInteraction.mjs';

function reportedRange(available, min, max, defaultValue, value) {
    return available === true && [min, max, defaultValue, value].every(Number.isSafeInteger) &&
        min <= max && defaultValue >= min && defaultValue <= max && value >= min && value <= max
        ? { min, max, defaultValue, value } : null;
}

export function bassControl(target) {
    const caps = target?.status?.bassCapabilities;
    const physicalId = target?.physicalId;
    if ([caps?.DeviceID, target?.status?.bass?.DeviceID].some(id => id && String(id).trim() !== physicalId)) return null;
    const range = reportedRange(caps?.BassAvailable, caps?.BassMin, caps?.BassMax,
        caps?.BassDefault, target?.status?.bass?.ActualBass);
    return range && target?.controlId && target?.physicalId ? range : null;
}

export function balanceControl(deviceId, device) {
    const coherent = projectedPairMaster(deviceId, device);
    const balance = device?.status?.balance;
    const range = reportedRange(balance?.Available, balance?.Min, balance?.Max,
        balance?.Default, balance?.Actual);
    return range && coherent ? { ...range, ...coherent.master, ...coherent.identity } : null;
}

// The server validates physical pair membership. Here we only locate the
// projected master and keep its identity for the admitted operation.
// The projection lists only registered members, so the peer can be missing.
function projectedPairMaster(deviceId, device) {
    const pair = device?.stereoPair;
    const targets = physicalSettingsTargets(deviceId, device);
    const master = targets.find(target => target.controlId === deviceId && target.physicalId === pair?.masterDeviceId);
    if (!pair?.id || targets.length > 2 || !master) return null;
    return { master, targets, identity: {
        name: pair.name || device?.info?.name || 'Stereo pair', pairId: pair.id,
        pairMembers: pair.members.map(member => [member.deviceId, member.role]).sort(),
    } };
}

function projectedPair(deviceId, device) {
    const pair = projectedPairMaster(deviceId, device);
    return pair?.targets.length === 2 ? pair : null;
}

export function pairBassControl(deviceId, device) {
    const coherent = projectedPair(deviceId, device);
    if (!coherent || !coherent.targets.every(target => target.status?.connectivity === 'online' &&
        Number.isSafeInteger(target.status?.epoch))) return null;
    const ranges = coherent.targets.map(bassControl);
    if (ranges.some(range => !range) || ['min', 'max', 'defaultValue'].some(key => ranges[0][key] !== ranges[1][key])) return null;
    const range = bassControl(coherent.master);
    return { ...range, ...coherent.master, ...coherent.identity,
        observationConfirmed: ranges.every(member => member.value === range.value),
        pairValues: ranges.map(member => member.value),
        bassMembers: coherent.targets.map(target => ({ controlId: target.controlId, physicalId: target.physicalId,
            epoch: target.status?.epoch, bassRevision: target.status?.bassRevision })),
    };
}

export function physicalSettingsTargets(deviceId, device) {
    if (!device?.stereoPair) {
        return deviceId ? [{ controlId: deviceId,
            physicalId: String(device?.info?.device_id || '').trim(),
            name: device?.info?.name || deviceId, role: '', status: device?.status }] : [];
    }
    return [...(device.settingsTargets || [])]
        .sort((a, b) => (a.role === 'LEFT' ? 0 : 1) - (b.role === 'LEFT' ? 0 : 1));
}

export function balanceLabel(value) {
    return value === 0 ? 'Centered' : value < 0 ? `Left ${-value}` : `Right ${value}`;
}

export function bassObservations(target) {
    return target.bassMembers ? target.bassMembers.map(member =>
        [member.controlId, member.physicalId, member.epoch, member.bassRevision])
        : [[target.controlId, target.physicalId, target.status?.epoch, target.status?.bassRevision]];
}

export async function writeSoundSetting(api, target, value) {
    try {
        await api[`${target.kind}Checked`](target.controlId, value);
    } catch (error) {
        if (error?.definitive) return { success: false, error: error.message };
        // A lost response does not prove rejection. Verify once, never replay.
    }
    let observations;
    try {
        const readback = await api.device(target.controlId);
        const device = readback?.data;
        const confirmation = target.kind === 'balance' ? balanceControl(target.controlId, device)
            : target.pairId ? pairBassControl(target.controlId, device)
                : bassControl({ controlId: target.controlId, physicalId: device?.info?.device_id,
                    status: device?.status });
        const sameConnection = Number.isSafeInteger(target.status?.epoch) && device?.status?.epoch === target.status.epoch;
        const currentBass = target.pairId ? confirmation : { controlId: target.controlId,
            physicalId: String(device?.info?.device_id || '').trim(), status: device?.status };
        const freshBass = target.kind !== 'bass' || observationsAdvanced(bassObservations(currentBass || {}),
            target.observations || bassObservations(target));
        const samePhysical = String(device?.info?.device_id || '').trim() === target.physicalId;
        const samePair = !target.pairId || (confirmation?.pairId === target.pairId &&
            JSON.stringify(confirmation.pairMembers) === JSON.stringify(target.pairMembers) &&
            (!target.bassMembers || confirmation.bassMembers?.length === target.bassMembers.length && confirmation.bassMembers.every((member, index) =>
                ['controlId', 'physicalId', 'epoch'].every(key => member[key] === target.bassMembers[index][key]))));
        if (target.kind === 'bass' && readback?.success && sameConnection && samePhysical && samePair) {
            observations = bassObservations(currentBass || {});
        }
        if (readback?.success && freshBass && sameConnection && samePhysical && samePair &&
            confirmation?.observationConfirmed !== false && confirmation?.value === value) {
            return { success: true, confirmedValue: value, ...(observations ? { observations } : {}) };
        }
    } catch (_) {
        // Read failures retain uncertainty; no mutation is repeated.
    }
    return { success: false, outcome: 'unverified', ...(observations ? { observations } : {}), error: 'Unverified: the speaker has not confirmed this sound setting. Refresh settings before retrying.' };
}
