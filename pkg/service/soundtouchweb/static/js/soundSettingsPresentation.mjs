function reportedRange(available, min, max, defaultValue, value) {
    return available === true && [min, max, defaultValue, value].every(Number.isSafeInteger) &&
        min <= max && defaultValue >= min && defaultValue <= max && value >= min && value <= max
        ? { min, max, defaultValue, value } : null;
}

export function bassControl(target) {
    const caps = target?.bassCapabilities;
    const range = reportedRange(caps?.BassAvailable, caps?.BassMin, caps?.BassMax,
        caps?.BassDefault, target?.status?.bass?.ActualBass);
    return range && target?.controlId && target?.physicalId ? range : null;
}

export function settingsBassReadback(status, snapshot) {
    if (!snapshot?.bass) return status?.bass;
    const epoch = status?.epoch;
    const revision = status?.revision;
    const newerProjection = Number.isSafeInteger(epoch) && Number.isSafeInteger(snapshot.statusEpoch) &&
        (epoch > snapshot.statusEpoch || (epoch === snapshot.statusEpoch &&
            Number.isSafeInteger(revision) && revision > snapshot.statusRevision));
    return newerProjection ? status?.bass : snapshot.bass;
}

export function balanceControl(deviceId, device) {
    const pair = device?.stereoPair;
    const targets = physicalSettingsTargets(deviceId, device);
    const master = targets.find(target => target.physicalId === pair?.masterDeviceId);
    const balance = device?.status?.balance;
    const range = reportedRange(balance?.Available, balance?.Min, balance?.Max,
        balance?.Default, balance?.Actual);
    if (!range || !pair?.id || !samePairGroup(pair, device?.status?.group) || targets.length !== 2 || !master ||
        !targets.every(target => samePairGroup(pair, target.status?.group)) ||
        !targets.some(target => target.role === 'LEFT') || !targets.some(target => target.role === 'RIGHT')) return null;
    return { ...range, ...master, name: pair.name || device?.info?.name || 'Stereo pair', pairId: pair.id,
        pairMembers: pair.members.map(member => [member.deviceId, member.role]).sort() };
}

function samePairGroup(pair, group) {
    if (!pair || !group || String(group.ID || '').trim() !== pair.id ||
        String(group.MasterDeviceID || '').trim() !== pair.masterDeviceId) return false;
    const roles = group.Roles?.Roles;
    if (!Array.isArray(roles) || roles.length !== 2 || pair.members?.length !== 2) return false;
    const members = roles.map(role => [String(role.DeviceID || '').trim(), String(role.Role || '').trim().toUpperCase()]);
    return new Set(members.map(member => member[0])).size === 2 &&
        members.some(member => member[1] === 'LEFT') && members.some(member => member[1] === 'RIGHT') &&
        JSON.stringify(members.sort()) === JSON.stringify(pair.members.map(member => [member.deviceId, member.role]).sort());
}

export function physicalSettingsTargets(deviceId, device) {
    const targets = Array.isArray(device?.settingsTargets) ? device.settingsTargets : [];
    const unique = targets.filter(target => target?.controlId && target?.physicalId &&
        targets.filter(other => other.controlId === target.controlId || other.physicalId === target.physicalId).length === 1);
    if (device?.stereoPair) {
        return unique.filter(target => device.stereoPair.members?.some(member =>
            member.deviceId === target.physicalId && member.role === target.role))
            .sort((a, b) => (a.role === 'LEFT' ? 0 : 1) - (b.role === 'LEFT' ? 0 : 1));
    }
    const physicalId = String(device?.info?.device_id || '').trim();
    return unique.filter(target => target.controlId === deviceId && target.physicalId === physicalId);
}

export function balanceLabel(value) {
    return value === 0 ? 'Centered' : value < 0 ? `Left ${-value}` : `Right ${value}`;
}

export async function writeSoundSetting(api, target, value) {
    let response;
    try {
        const write = api[`${target.kind}Checked`] || api[target.kind];
        response = await write(target.controlId, value);
    } catch (error) {
        if (error?.definitive) return { success: false, error: error.message };
        // Losing the response does not prove the speaker rejected the write.
        // Read once to establish the state; never replay the mutation.
        response = { success: false, outcome: 'unverified' };
    }
    if (response?.success === false && response.outcome !== 'unverified') return response;
    try {
        const readback = target.kind === 'bass'
            ? await api.settings(target.controlId) : await api.device(target.controlId);
        const data = readback?.data;
        const epoch = target.kind === 'bass' ? data?.statusEpoch : data?.status?.epoch;
        const sameConnection = Number.isSafeInteger(target.status?.epoch) && epoch === target.status.epoch;
        const confirmation = target.kind === 'bass' ? bassControl({ controlId: target.controlId,
            physicalId: data?.targetIdentity, bassCapabilities: data?.bassCapabilities, status: { bass: data?.bass } })
            : balanceControl(target.controlId, data);
        const sameTarget = target.kind === 'bass' ? data?.targetIdentity === target.physicalId
            : confirmation?.physicalId === target.physicalId && confirmation?.pairId === target.pairId &&
                JSON.stringify(confirmation.pairMembers) === JSON.stringify(target.pairMembers);
        if (readback?.success && !data?.errors?.bass && sameConnection && sameTarget && confirmation?.value === value) {
            return { success: true, confirmedValue: confirmation.value };
        }
    } catch (_) {
        // An accepted POST is not retried when the confirmation read fails.
    }
    return { success: false, outcome: 'unverified', error: 'Unverified: the speaker has not confirmed this sound setting. Refresh settings before retrying.' };
}
