export const SETTINGS_SECTION_ORDER = [
    'clock',
    'standby',
    'language',
    'sync',
    'bluetooth',
    'sources',
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
    const controlId = String(master?.ipAddress || '').trim();

    if (!masterDeviceId || !controlId) return null;

    return {
        controlId,
        physicalId: masterDeviceId,
        name: master.name || controlId,
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
        wifiOnboarding: Boolean(support.wifiOnboarding && snapshot?.onboardingUrl),
    };

    return SETTINGS_SECTION_ORDER.filter(section => visible[section]);
}
