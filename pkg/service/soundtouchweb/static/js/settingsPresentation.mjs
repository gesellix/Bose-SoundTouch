export const SETTINGS_SECTION_ORDER = [
    'clock',
    'standby',
    'language',
    'sync',
    'bluetooth',
    'sources',
    'network',
    'wifiOnboarding',
];

export function deviceSettingsTitle(name, role = '') {
    const target = typeof name === 'string' ? name.trim() : '';
    const targetRole = typeof role === 'string' ? role.trim().toUpperCase() : '';
    const identity = [targetRole, target].filter(Boolean).join(' · ');
    return identity ? `Device settings · ${identity}` : 'Device settings';
}

export function deviceSettingsTarget(deviceId, device) {
    const fallbackId = typeof deviceId === 'string' ? deviceId.trim() : '';
    const pair = device?.stereoPair;

    if (!pair) {
        return fallbackId ? {
            controlId: fallbackId,
            physicalId: String(device?.info?.device_id || '').trim() || fallbackId,
            name: device?.info?.name || fallbackId,
            role: '',
        } : null;
    }

    const masterDeviceId = String(pair.masterDeviceId || '').trim();
    const master = Array.isArray(pair.members)
        ? pair.members.find(member => String(member?.deviceId || '').trim() === masterDeviceId)
        : null;

    // A pair is listed under its master's registry key, which is the page's
    // deviceId. The member's ipAddress is what the speaker reports about
    // itself; it differs from the key for a speaker registered by hostname or
    // one with a second interface, and the service does not know it as an id.
    if (!fallbackId || !masterDeviceId || !master) return null;

    return {
        controlId: fallbackId,
        physicalId: masterDeviceId,
        name: master.name || fallbackId,
        role: String(master.role || '').trim().toUpperCase(),
    };
}

function hasError(errors, keys) {
    if (!errors || typeof errors !== 'object') return false;
    return keys.some(key => Boolean(errors[key]));
}

function hasOwn(value, key) {
    return value != null && Object.prototype.hasOwnProperty.call(value, key);
}

export function clockControls(snapshot) {
    const support = snapshot?.support || {};
    const display = snapshot?.clockDisplay;
    const time = snapshot?.clockTime;
    const displayReadback = Boolean(support.clockDisplay && display);

    return {
        display: displayReadback && hasOwn(display, 'enabled') && typeof display.enabled === 'boolean',
        format: displayReadback && ['12', '24', 'auto'].includes(display.format),
        timeZone: displayReadback && typeof display.timeZone === 'string' && display.timeZone.trim() !== '',
        currentTime: Boolean(
            support.clockTime && time && (
                (Number.isFinite(time.utc) && time.utc > 0)
                || (typeof time.value === 'string' && time.value.trim() !== '')
            ),
        ),
    };
}

export function clockDisplayPatch(snapshot, field, value) {
    const controls = clockControls(snapshot);

    if (field === 'enabled' && controls.display && typeof value === 'boolean') {
        return { enabled: value };
    }
    if (field === 'format' && controls.format && ['12', '24', 'auto'].includes(value)) {
        return { format: value };
    }
    if (field === 'timeZone' && controls.timeZone && typeof value === 'string' && value.trim() !== '') {
        return { timeZone: value.trim() };
    }

    return null;
}

// withPendingView lays a change that is still being applied over the last
// confirmed snapshot, one setting group deep, so a control keeps showing what
// the user chose instead of snapping back until the outcome arrives.
export function withPendingView(snapshot, pending) {
    if (!snapshot || !pending || typeof pending !== 'object') return snapshot;

    const view = { ...snapshot };
    for (const [key, value] of Object.entries(pending)) {
        if (snapshot[key] && typeof snapshot[key] === 'object' && value && typeof value === 'object') {
            view[key] = { ...snapshot[key], ...value };
        }
    }
    return view;
}

export function networkInterfaceGroups(network) {
    const interfaces = Array.isArray(network?.interfaces) ? network.interfaces : [];
    const isConnected = item => item.state === 'NETWORK_WIFI_CONNECTED'
        || item.state === 'NETWORK_ETHERNET_CONNECTED';
    return {
        connected: interfaces.filter(isConnected),
        disconnected: interfaces.filter(item => !isConnected(item)),
    };
}

export function networkInterfaceName(item, technical = false) {
    if (item.type === 'WIFI_INTERFACE') {
        if (technical) return item.name ? `Wi-Fi interface · ${item.name}` : 'Wi-Fi interface';
        return item.ssid ? `Wi-Fi · ${item.ssid}` : 'Wi-Fi';
    }
    if (item.type === 'ETHERNET_INTERFACE') {
        if (technical) return item.name ? `Ethernet interface · ${item.name}` : 'Ethernet interface';
        return 'Ethernet';
    }
    return item.name || item.type || 'Network interface';
}

export function networkInterfaceSummary(item, technical = false) {
    if (technical) {
        return [item.state?.endsWith('_DISCONNECTED') ? 'Not connected' : 'Connection state not reported',
            item.ipAddress, item.band].filter(Boolean).join(' · ');
    }
    return [item.ipAddress || 'IP not reported', item.band].filter(Boolean).join(' · ');
}

export function settingsSections(snapshot) {
    const support = snapshot?.support || {};
    const errors = snapshot?.errors;
    const clock = clockControls(snapshot);
    const visible = {
        clock: Object.values(clock).some(Boolean) || hasError(errors, ['clock', 'clockDisplay', 'clockTime']),
        standby: Boolean(support.systemTimeout && snapshot?.systemTimeout) ||
            hasError(errors, ['standby', 'systemTimeout']),
        language: Boolean(support.language && snapshot?.language) || hasError(errors, ['language']),
        sync: Boolean(support.sync && snapshot?.sync) || hasError(errors, ['sync']),
        bluetooth: Boolean(
            support.bluetooth || support.bluetoothPair || support.bluetoothClear,
        ) || hasError(errors, ['bluetooth', 'bluetoothPair', 'bluetoothClear']),
        sources: Boolean(support.sourceNaming && snapshot?.sources?.length) ||
            hasError(errors, ['sources', 'sourceNaming']),
        network: snapshot?.network != null || hasError(errors, ['network']),
        wifiOnboarding: Boolean(support.wifiOnboarding && snapshot?.onboardingUrl),
    };

    return SETTINGS_SECTION_ORDER.filter(section => visible[section]);
}
