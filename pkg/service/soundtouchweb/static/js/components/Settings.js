import { h } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import {
    clockControls,
    clockDisplayPatch,
    deviceSettingsTitle,
    settingsSections,
} from '../settingsPresentation.mjs';

const html = htm.bind(h);

const SECTION_ERROR_KEYS = {
    clock: ['clock', 'clockDisplay', 'clockTime'],
    standby: ['standby', 'systemTimeout'],
    language: ['language'],
    sync: ['sync'],
    // bluetoothClear is not listed: the API sets it only to explain an
    // unverified clear, which the section's own result already shows.
    bluetooth: ['bluetooth', 'bluetoothPair'],
    sources: ['sources', 'sourceNaming'],
};

function errorText(error, fallback) {
    if (typeof error === 'string' && error) return error;
    if (typeof error?.message === 'string' && error.message) return error.message;
    return fallback;
}

function snapshotErrors(errors, section) {
    if (!errors) return [];
    if (typeof errors === 'string') return [errors];
    if (Array.isArray(errors)) {
        return errors.map(error => errorText(error, '')).filter(Boolean);
    }

    return (SECTION_ERROR_KEYS[section] || [section]).flatMap(key => {
        const value = errors[key];
        if (!value) return [];
        return (Array.isArray(value) ? value : [value])
            .map(error => errorText(error, ''))
            .filter(Boolean);
    });
}

function SectionErrors({ snapshot, section }) {
    const messages = snapshotErrors(snapshot?.errors, section);
    if (messages.length === 0) return null;

    return html`
        <div class="settings-error" role="alert">
            ${messages.map((message, index) => html`<div key=${`${section}-${index}`}>${message}</div>`)}
        </div>
    `;
}

function SectionResult({ result }) {
    if (!result) return null;

    const role = result.kind === 'busy' || result.kind === 'success' ? 'status' : 'alert';
    return html`
        <div class=${`settings-result settings-result-${result.kind}`} role=${role}>
            ${result.message}
        </div>
    `;
}

function Toggle({ checked, disabled, label, onChange }) {
    return html`
        <label class="settings-toggle">
            <input
                type="checkbox"
                checked=${Boolean(checked)}
                disabled=${disabled}
                onChange=${event => onChange(event.target.checked)}
            />
            <span class="settings-toggle-track" aria-hidden="true"></span>
            <span>${label}</span>
        </label>
    `;
}

function SourceRenameRow({ source, busy, onRename }) {
    const [name, setName] = useState(source.displayName || '');

    useEffect(() => {
        setName(source.displayName || '');
    }, [source.displayName, source.source, source.sourceAccount]);

    const trimmedName = name.trim();
    const unchanged = trimmedName === (source.displayName || '');

    function submit(event) {
        event.preventDefault();
        if (!trimmedName || unchanged || busy) return;
        onRename(source.source, source.sourceAccount || '', trimmedName);
    }

    return html`
        <form class="settings-source-row" onSubmit=${submit}>
            <label>
                <span>${source.source}${source.sourceAccount ? ` · ${source.sourceAccount}` : ''}</span>
                <input
                    type="text"
                    value=${name}
                    disabled=${busy}
                    onInput=${event => setName(event.target.value)}
                    aria-label=${`${source.source} display name`}
                />
            </label>
            <button
                type="submit"
                class="btn-secondary settings-action"
                disabled=${busy || !trimmedName || unchanged}
            >Rename</button>
        </form>
    `;
}

export function Settings({ deviceId, targetIdentity = '', targetName = '', targetRole = '' }) {
    const expectedIdentity = String(targetIdentity || '').trim();
    const targetGeneration = `${deviceId}\u0000${expectedIdentity}`;
    const [snapshotState, setSnapshotState] = useState({ targetGeneration, data: null });
    const [loading, setLoading] = useState(false);
    const [loadError, setLoadError] = useState('');
    const [busy, setBusy] = useState('');
    const [actionResults, setActionResults] = useState({});
    const requested = useRef(false);
    const detailsRef = useRef(null);
    const resetTarget = useRef(targetGeneration);
    const loadGeneration = useRef(0);
    const generationTarget = useRef(targetGeneration);
    const snapshot = snapshotState.targetGeneration === targetGeneration ? snapshotState.data : null;

    if (generationTarget.current !== targetGeneration) {
        generationTarget.current = targetGeneration;
        loadGeneration.current += 1;
    }

    useEffect(() => {
        // Only a real target change resets. On mount there is nothing to
        // reset, and the effect can run after the first load has already
        // started, which a reset would orphan.
        if (resetTarget.current === targetGeneration) return;
        resetTarget.current = targetGeneration;

        setSnapshotState({ targetGeneration, data: null });
        setLoading(false);
        setLoadError('');
        setBusy('');
        setActionResults({});
        requested.current = false;
        // An open section that changes target will not see another toggle
        // event, so load the new target now instead of leaving it empty.
        if (detailsRef.current?.open) load();
    }, [targetGeneration]);

    function checkedSnapshot(data) {
        if (!data) return null;
        if (String(data.targetIdentity || '').trim() !== expectedIdentity) {
            throw new Error('The physical speaker changed. Reload device details before changing settings.');
        }
        return data;
    }

    async function load() {
        if (requested.current) return;
        const generation = loadGeneration.current;
        requested.current = true;
        setLoading(true);
        setLoadError('');
        try {
            if (!expectedIdentity) {
                throw new Error('The physical speaker identity is unavailable.');
            }
            const response = await api.settings(deviceId);
            if (generation !== loadGeneration.current) return;
            if (!response?.success || !response.data) {
                throw new Error(response?.error || 'The speaker did not return its settings.');
            }
            setSnapshotState({ targetGeneration, data: checkedSnapshot(response.data) });
        } catch (error) {
            if (generation !== loadGeneration.current) return;
            setLoadError(errorText(error, 'Could not load settings. Check that the speaker is reachable.'));
            requested.current = false;
        } finally {
            if (generation === loadGeneration.current) setLoading(false);
        }
    }

    async function mutate(section, action, fallback) {
        if (busy) return;
        const snapshotIdentity = String(snapshot?.targetIdentity || '').trim();
        if (!snapshotIdentity || snapshotIdentity !== expectedIdentity) {
            setActionResults(previous => ({
                ...previous,
                [section]: { kind: 'error', message: 'Reload settings before changing this speaker.' },
            }));
            return;
        }
        const generation = loadGeneration.current;
        setBusy(section);
        setActionResults(previous => ({
            ...previous,
            [section]: { kind: 'busy', message: 'Applying change…' },
        }));
        try {
            const response = await action(snapshotIdentity);
            if (generation !== loadGeneration.current) return;
            if (response?.outcome === 'unverified') {
                if (response.data) {
                    setSnapshotState({ targetGeneration, data: checkedSnapshot(response.data) });
                } else {
                    // Nothing came back to show what the speaker holds now;
                    // re-read it rather than keep presenting the old values.
                    setSnapshotState({ targetGeneration, data: null });
                    requested.current = false;
                    await load();
                    if (generation !== loadGeneration.current) return;
                }
                const message = response.error || 'The speaker accepted the command, but its result could not be verified.';
                setActionResults(previous => ({
                    ...previous,
                    [section]: { kind: 'unverified', message },
                }));
                return;
            }
            if (response?.success && response.outcome === 'confirmed') {
                if (response.data) {
                    setSnapshotState({ targetGeneration, data: checkedSnapshot(response.data) });
                } else {
                    setSnapshotState({ targetGeneration, data: null });
                    requested.current = false;
                    await load();
                    if (generation !== loadGeneration.current) return;
                }
                setActionResults(previous => ({
                    ...previous,
                    [section]: {
                        kind: 'success',
                        message: response.warning || 'Change applied and verified.',
                    },
                }));
                return;
            }
            if (!response?.success || !response.data || response.error) {
                throw new Error(response?.error || fallback);
            }
            setSnapshotState({ targetGeneration, data: checkedSnapshot(response.data) });
            setActionResults(previous => ({
                ...previous,
                [section]: { kind: 'success', message: 'Change applied and verified.' },
            }));
        } catch (error) {
            if (generation !== loadGeneration.current) return;
            const message = errorText(error, fallback);
            setActionResults(previous => ({
                ...previous,
                [section]: { kind: 'error', message },
            }));
        } finally {
            if (generation === loadGeneration.current) setBusy('');
        }
    }

    function onToggle(event) {
        if (event.currentTarget.open && !snapshot && !loading) load();
    }

    const sections = settingsSections(snapshot);
    const support = snapshot?.support || {};
    const clock = snapshot?.clockDisplay || {};
    const clockControl = clockControls(snapshot);
    const browserTimeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

    function updateClockDisplay(field, value, fallback) {
        const patch = clockDisplayPatch(snapshot, field, value);
        if (!patch) return;
        mutate('clock', targetIdentity => api.setClockDisplay(deviceId, targetIdentity, patch), fallback);
    }

    return html`
        <details class="settings-section" ref=${detailsRef} onToggle=${onToggle}>
            <summary class="settings-summary">
                <span class="section-title">${deviceSettingsTitle(targetName, targetRole)}</span>
                <span class="settings-chevron" aria-hidden="true"></span>
            </summary>

            <div class="settings-content">
                ${loading ? html`<div class="settings-loading" role="status">Loading settings…</div>` : null}
                ${loadError ? html`
                    <div class="settings-load-error" role="alert">
                        <span>${loadError}</span>
                        <button class="btn-secondary settings-action" onClick=${load}>Retry</button>
                    </div>
                ` : null}
                ${!snapshot ? Object.entries(actionResults).map(([section, result]) => html`
                    <${SectionResult} key=${section} result=${result} />
                `) : null}

                ${snapshot && sections.length === 0 ? html`
                    <p class="settings-empty">This speaker does not report supported settings.</p>
                ` : null}

                ${sections.includes('clock') ? html`
                    <section class="settings-group">
                        <h4>Clock</h4>
                        <${SectionErrors} snapshot=${snapshot} section="clock" />
                        <${SectionResult} result=${actionResults.clock} />
                        ${clockControl.display ? html`
                            <${Toggle}
                                checked=${clock.enabled}
                                disabled=${Boolean(busy)}
                                label="Show clock display"
                                onChange=${enabled => updateClockDisplay(
                                    'enabled',
                                    enabled,
                                    'Could not update the clock display.',
                                )}
                            />
                        ` : null}
                        ${clockControl.format ? html`
                            <label class="settings-field">
                                <span>Time format</span>
                                <select
                                    value=${clock.format}
                                    disabled=${Boolean(busy)}
                                    onChange=${event => updateClockDisplay(
                                        'format',
                                        event.target.value,
                                        'Could not update the time format.',
                                    )}
                                >
                                    <option value="12">12-hour</option>
                                    <option value="24">24-hour</option>
                                    <option value="auto">Automatic</option>
                                </select>
                            </label>
                        ` : null}
                        ${clockControl.timeZone ? html`
                            <div class="settings-command-row">
                                <div>
                                    <span class="settings-command-label">Timezone</span>
                                    <span class="settings-value">${clock.timeZone}</span>
                                </div>
                                <button
                                    class="btn-secondary settings-action"
                                    disabled=${Boolean(busy) || !browserTimeZone}
                                    onClick=${() => updateClockDisplay(
                                        'timeZone',
                                        browserTimeZone,
                                        'Could not update the timezone.',
                                    )}
                                >Use browser timezone</button>
                            </div>
                        ` : null}
                        ${clockControl.currentTime ? html`
                            <button
                                class="btn-secondary settings-action"
                                disabled=${Boolean(busy)}
                                onClick=${() => mutate(
                                    'clock',
                                    targetIdentity => api.setClockTime(deviceId, targetIdentity),
                                    'Could not set the speaker clock.',
                                )}
                            >Set clock now</button>
                        ` : null}
                    </section>
                ` : null}

                ${sections.includes('standby') ? html`
                    <section class="settings-group">
                        <h4>Automatic standby</h4>
                        <${SectionErrors} snapshot=${snapshot} section="standby" />
                        <${SectionResult} result=${actionResults.standby} />
                        ${snapshot.systemTimeout ? html`
                            <${Toggle}
                                checked=${snapshot.systemTimeout.enabled}
                                disabled=${Boolean(busy)}
                                label="Enter standby automatically when idle"
                                onChange=${enabled => mutate(
                                    'standby',
                                    targetIdentity => api.setSystemTimeout(deviceId, targetIdentity, enabled),
                                    'Could not update automatic standby.',
                                )}
                            />
                        ` : null}
                    </section>
                ` : null}

                ${sections.includes('language') ? html`
                    <section class="settings-group">
                        <h4>Language</h4>
                        <${SectionErrors} snapshot=${snapshot} section="language" />
                        <${SectionResult} result=${actionResults.language} />
                        ${snapshot.language ? html`
                            <label class="settings-field">
                                <span>System language</span>
                                <select
                                    value=${String(snapshot.language.code)}
                                    disabled=${Boolean(busy)}
                                    onChange=${event => mutate(
                                        'language',
                                        targetIdentity => api.setLanguage(
                                            deviceId, targetIdentity, Number(event.target.value)),
                                        'Could not update the system language.',
                                    )}
                                >
                                    ${(snapshot.language.options || []).map(option => html`
                                        <option key=${option.code} value=${String(option.code)}>${option.name}</option>
                                    `)}
                                </select>
                            </label>
                        ` : null}
                    </section>
                ` : null}

                ${sections.includes('sync') ? html`
                    <section class="settings-group">
                        <h4>Audio sync</h4>
                        <${SectionErrors} snapshot=${snapshot} section="sync" />
                        <${SectionResult} result=${actionResults.sync} />
                        ${snapshot.sync ? html`
                            <fieldset class="settings-segmented" disabled=${Boolean(busy)}>
                                <legend>Playback mode</legend>
                                ${[
                                    ['SYNC_TO_ROOM', 'Video'],
                                    ['SYNC_TO_ZONE', 'Multi-room'],
                                ].map(([mode, label]) => html`
                                    <label key=${mode} class=${snapshot.sync.mode === mode ? 'selected' : ''}>
                                        <input
                                            type="radio"
                                            name=${`sync-mode-${deviceId}`}
                                            value=${mode}
                                            checked=${snapshot.sync.mode === mode}
                                            onChange=${() => mutate(
                                                'sync',
                                                targetIdentity => api.setSync(deviceId, targetIdentity, mode),
                                                'Could not update audio sync.',
                                            )}
                                        />
                                        <span>${label}</span>
                                    </label>
                                `)}
                            </fieldset>
                        ` : null}
                    </section>
                ` : null}

                ${sections.includes('bluetooth') ? html`
                    <section class="settings-group">
                        <h4>Bluetooth</h4>
                        <${SectionErrors} snapshot=${snapshot} section="bluetooth" />
                        <${SectionResult} result=${actionResults.bluetooth} />
                        ${support.bluetooth && snapshot.bluetooth ? html`
                            <dl class="settings-data-list">
                                <div><dt>Status</dt><dd>${snapshot.bluetooth.connectionStatus || 'Not reported'}</dd></div>
                                ${snapshot.bluetooth.deviceName ? html`<div><dt>Device</dt><dd>${snapshot.bluetooth.deviceName}</dd></div>` : null}
                                ${snapshot.bluetooth.macAddress ? html`<div><dt>MAC address</dt><dd>${snapshot.bluetooth.macAddress}</dd></div>` : null}
                            </dl>
                        ` : null}
                        ${support.bluetoothPair ? html`
                            <button
                                class="btn-secondary settings-action"
                                disabled=${Boolean(busy)}
                                onClick=${() => mutate(
                                    'bluetooth',
                                    targetIdentity => api.bluetoothPair(deviceId, targetIdentity),
                                    'Could not enter Bluetooth pairing mode.',
                                )}
                            >Enter pairing mode</button>
                        ` : null}
                        ${support.bluetoothClear ? html`
                            <div class="settings-danger-zone">
                                <button
                                    class="btn-secondary settings-action settings-danger-action"
                                    disabled=${Boolean(busy)}
                                    onClick=${() => {
                                        if (!confirm('Clear all Bluetooth pairings from this speaker?')) return;
                                        mutate(
                                            'bluetooth',
                                            targetIdentity => api.clearBluetoothPairings(deviceId, targetIdentity),
                                            'Could not clear Bluetooth pairings.',
                                        );
                                    }}
                                >Clear all pairings</button>
                            </div>
                        ` : null}
                    </section>
                ` : null}

                ${sections.includes('sources') ? html`
                    <section class="settings-group">
                        <h4>Source names</h4>
                        <${SectionErrors} snapshot=${snapshot} section="sources" />
                        <${SectionResult} result=${actionResults.sources} />
                        <div class="settings-source-list">
                            ${(snapshot.sources || []).map(source => html`
                                <${SourceRenameRow}
                                    key=${`${source.source}-${source.sourceAccount || ''}`}
                                    source=${source}
                                    busy=${Boolean(busy)}
                                    onRename=${(sourceName, sourceAccount, name) => mutate(
                                        'sources',
                                        targetIdentity => api.setSourceName(
                                            deviceId, targetIdentity, sourceName, sourceAccount, name),
                                        'Could not rename the source.',
                                    )}
                                />
                            `)}
                        </div>
                    </section>
                ` : null}

                ${sections.includes('wifiOnboarding') ? html`
                    <section class="settings-group">
                        <h4>Wi-Fi setup</h4>
                        <a
                            class="btn-secondary settings-action settings-command-link"
                            href=${snapshot.onboardingUrl}
                            target="_blank"
                            rel="noopener"
                        >Open Wi-Fi setup</a>
                    </section>
                ` : null}

            </div>
        </details>
    `;
}
