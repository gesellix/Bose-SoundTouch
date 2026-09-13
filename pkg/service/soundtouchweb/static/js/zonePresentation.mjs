function count(value, fallback = 0) {
    return Number.isInteger(value) && value >= 0 ? value : fallback;
}

function physicalMembers(zone) {
    return (zone?.members || []).flatMap(member => member?.physicalMembers || []);
}

function connectivityPresentation(member) {
    const reported = member?.connectivity;
    const connectivity = ['online', 'stale', 'offline'].includes(reported)
        ? reported
        : (member?.available ? 'online' : 'offline');

    return {
        connectivity,
        connectivityLabel: connectivity.charAt(0).toUpperCase() + connectivity.slice(1),
    };
}

function memberIPAddress(member) {
    const direct = member?.ip || member?.controlId || '';
    if (member?.ip && member.ip !== member.controlId) return member.ip;

    const physical = member?.physicalMembers || [];
    const representative = physical.find(candidate => candidate.deviceId === member?.hwId) || physical[0];

    return representative?.ip || direct;
}

function addIdentifier(ids, value) {
    if (typeof value === 'string' && value.trim()) ids.add(value.trim());
}

export function zoneMemberIdentifiers(...members) {
    const ids = new Set();

    for (const member of members.flat().filter(Boolean)) {
        addIdentifier(ids, member.controlId);
        addIdentifier(ids, member.ip);
        addIdentifier(ids, member.hwId);
        addIdentifier(ids, member.deviceId);
        for (const deviceId of member.deviceIds || []) addIdentifier(ids, deviceId);
        for (const physical of member.physicalMembers || []) {
            addIdentifier(ids, physical?.deviceId);
            addIdentifier(ids, physical?.ip);
        }
        for (const physical of member.stereoPair?.members || []) {
            addIdentifier(ids, physical?.deviceId);
            addIdentifier(ids, physical?.ipAddress);
        }
    }

    return ids;
}

export function zoneTopologyFingerprint(zone) {
    if (!zone) return '';

    return JSON.stringify([
        zone.masterControlId || '',
        zone.masterDeviceId || '',
        (zone.members || []).map(member => [
            member?.controlId || '',
            member?.hwId || '',
            ...(member?.deviceIds || []),
        ]),
    ]);
}

export function effectiveZoneDetail(restZone, projection, topLevelDevice, deviceId) {
    if (projection) {
        const members = projection.members || [];
        const master = members.find(member =>
            member?.controlId === projection.masterControlId ||
            member?.deviceIds?.includes(projection.masterDeviceId));
        const selected = members.find(member => member?.controlId === deviceId);
        const isMaster = projection.masterControlId === deviceId;
        if (!isMaster && !selected) return null;

        return {
            masterIp: projection.masterControlId,
            masterHwId: projection.masterDeviceId,
            masterName: master?.name || '',
            master,
            members: members.filter(member => member !== master),
            physicalMemberCount: projection.physicalMemberCount,
            isMaster,
            isSlave: !isMaster && Boolean(selected),
            isStandalone: false,
        };
    }

    // The inventory is the canonical logical projection. Once it publishes a
    // top-level device with no cached zone, stale REST management state must
    // not survive an externally initiated dissolution.
    if (topLevelDevice && topLevelDevice.status?.zone == null) {
        return {
            masterIp: '',
            masterHwId: '',
            masterName: '',
            master: null,
            members: [],
            physicalMemberCount: 1,
            isMaster: false,
            isSlave: false,
            isStandalone: true,
        };
    }

    return restZone;
}

export function zoneCardPresentation(zone) {
    const members = zone?.members || [];
    const memberCount = count(zone?.memberCount, members.length);
    const availableCount = count(
        zone?.availableMemberCount,
        members.filter(member => member?.available).length,
    );
    const degraded = Boolean(zone?.degraded || availableCount < memberCount);
    const physical = physicalMembers(zone);
    const physicalCount = count(zone?.physicalMemberCount, physical.length || memberCount);
    const availablePhysicalCount = physical.length > 0
        ? physical.filter(member => member?.available).length
        : physicalCount;

    let availabilityLabel = '';
    let availabilityTitle = `All ${memberCount} available`;
    if (degraded && availableCount < memberCount) {
        availabilityLabel = `${availableCount}/${memberCount} available`;
        availabilityTitle = `${memberCount - availableCount} unavailable`;
    } else if (degraded && availablePhysicalCount < physicalCount) {
        availabilityLabel = `${availablePhysicalCount}/${physicalCount} speakers available`;
        availabilityTitle = `${physicalCount - availablePhysicalCount} physical speaker unavailable`;
    } else if (degraded) {
        availabilityLabel = 'Degraded';
        availabilityTitle = 'The speaker topology reports a degraded state';
    }

    const health = degraded ? 'degraded' : 'healthy';

    return {
        groupLabel: `Group · ${memberCount}`,
        availabilityLabel,
        availabilityTitle,
        health,
        healthLabel: degraded ? `Degraded group: ${availabilityTitle}` : 'Healthy group',
    };
}

export function zoneMemberCountSummary(logicalCount, physicalCount) {
    const logical = count(logicalCount);
    const physical = count(physicalCount, logical);
    const logicalLabel = `${logical} ${logical === 1 ? 'member' : 'members'}`;

    if (logical === physical) return logicalLabel;

    return `${logicalLabel} · ${physical} ${physical === 1 ? 'speaker' : 'speakers'}`;
}

export function zoneMemberMetadata(member) {
    const connectivity = connectivityPresentation(member);
    const name = member?.name || member?.controlId || member?.ip || 'Unknown member';

    return {
        ...connectivity,
        name,
        modelType: member?.model || member?.type || 'Unknown model',
        ip: memberIPAddress(member),
        kind: member?.kind === 'stereoPair' ? 'Stereo pair' : 'Speaker',
        statusAriaLabel: `${name}: ${connectivity.connectivityLabel}`,
    };
}

export function physicalMemberMetadata(member) {
    const metadata = zoneMemberMetadata(member);

    return {
        ...metadata,
        role: (member?.role || 'Member').toUpperCase(),
    };
}
