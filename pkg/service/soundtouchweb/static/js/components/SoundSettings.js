import { h } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { bassControl, pairBassControl, balanceControl, physicalSettingsTargets, settingsBassReadback } from '../soundSettingsPresentation.mjs';
import { SteppedSoundControl } from './SteppedSoundControl.js';

const html = htm.bind(h);

export function SoundSettings({ deviceId, device, nested = false, active = false }) {
    const [open, setOpen] = useState(false);
    const [snapshots, setSnapshots] = useState({});
    const [loadError, setLoadError] = useState('');
    const [retry, setRetry] = useState(0);
    const ownership = useRef({ key: '', generation: 0 });
    const lastPresentation = useRef({ key: '', bass: {}, pairBass: null, balance: null });
    const targets = physicalSettingsTargets(deviceId, device);
    const ownerKey = JSON.stringify([deviceId, device?.stereoPair?.id || '', device?.stereoPair?.masterDeviceId || '',
        targets.map(target => [target.controlId, target.physicalId, target.role, target.status?.epoch])]);
    if (ownership.current.key !== ownerKey) ownership.current = {
        key: ownerKey, generation: ownership.current.generation + 1,
    };
    const generation = ownership.current.generation;
    if (lastPresentation.current.key !== ownerKey) lastPresentation.current = { key: ownerKey, bass: {}, pairBass: null, balance: null };
    useEffect(() => {
        if (!(nested ? active : open)) return;
        let disposed = false;
        setLoadError('');
        Promise.all(targets.map(async target => {
            const response = await api.settings(target.controlId);
            if (!response?.success || response.data?.targetIdentity !== target.physicalId ||
                (response.data?.statusEpoch !== undefined && response.data.statusEpoch !== target.status?.epoch)) {
                throw new Error(response?.error || 'The physical speaker changed. Reload settings.');
            }
            return [target.controlId, response.data];
        })).then(entries => {
            if (disposed || ownership.current.generation !== generation) return;
            setSnapshots({ ownerKey, data: Object.fromEntries(entries) });
        }, error => {
            if (!disposed && ownership.current.generation === generation) setLoadError(error.message || 'Sound settings could not be loaded.');
        });
        return () => { disposed = true; };
    }, [ownerKey, nested ? active : open, retry]);
    const bassTargets = targets.map(physical => {
        const snapshot = snapshots.ownerKey === ownerKey ? snapshots.data[physical.controlId] : null;
        const target = { ...physical, bassCapabilities: snapshot?.bassCapabilities,
            status: { ...physical.status, bass: settingsBassReadback(physical.status, snapshot) } };
        const current = snapshot?.errors?.bass ? null : bassControl(target);
        const presentationKey = `${target.controlId}:${target.physicalId}`;
        if (current) lastPresentation.current.bass[presentationKey] = current;
        return { target, control: current || lastPresentation.current.bass[presentationKey],
            valid: Boolean(current), error: snapshot?.errors?.bass };
    });
    const currentBalance = balanceControl(deviceId, device);
    if (currentBalance) lastPresentation.current.balance = currentBalance;
    const balance = currentBalance || lastPresentation.current.balance;
    const paired = Boolean(device?.stereoPair);
    const currentPairBass = paired && bassTargets.every(entry => entry.valid)
        ? pairBassControl(deviceId, { ...device, settingsTargets: bassTargets.map(entry => entry.target) }) : null;
    if (currentPairBass) lastPresentation.current.pairBass = currentPairBass;
    const pairBass = currentPairBass || lastPresentation.current.pairBass;
    const content = html`
        <div class="settings-content sound-settings-content">
            ${loadError ? html`<div class="settings-error" role="alert">${loadError}
                <button type="button" class="btn-secondary" onClick=${() => setRetry(value => value + 1)}>Retry</button></div>` : null}
            ${!paired ? bassTargets.filter(entry => entry.control).map(({ target, control, valid }) => html`<${SteppedSoundControl}
                key=${`bass:${target.controlId}:${target.physicalId}`} kind="bass"
                target=${target} control=${control} ownerKey=${ownerKey} available=${valid} />`) : null}
            ${paired && pairBass ? html`<${SteppedSoundControl}
                key=${`pair-bass:${pairBass.controlId}:${pairBass.pairId}`} kind="bass"
                target=${pairBass} control=${pairBass} ownerKey=${ownerKey} available=${Boolean(currentPairBass)} />` : null}
            ${paired && !currentPairBass && (pairBass || bassTargets.some(entry => entry.control))
                ? html`<div class="sound-control-readback">Pair bass requires both speakers to report matching settings and a confirmed stereo pair.</div>` : null}
            ${currentPairBass?.observationConfirmed === false
                ? html`<div class="sound-control-readback">The speakers currently report different bass values.</div>` : null}
            ${balance ? html`<${SteppedSoundControl} key=${`balance:${balance.controlId}:${balance.pairId}`}
                kind="balance" target=${{ ...balance, status: device.status }} control=${balance} ownerKey=${ownerKey}
                available=${Boolean(currentBalance)} />` : null}
            ${bassTargets.filter(entry => entry.error).map(({ target, error }) => html`<div class="settings-error" role="alert">
                Bass · ${target.name}: ${error}
                <button type="button" class="btn-secondary" onClick=${() => setRetry(value => value + 1)}>Retry</button></div>`)}
            ${!balance && !bassTargets.some(entry => entry.control) && !loadError && snapshots.ownerKey === ownerKey
                ? html`<div class="sound-control-readback">No supported sound controls reported.</div>` : null}
        </div>
    `;
    return nested ? html`<section class="sound-settings"><div class="section-title">Sound</div>${content}</section>` : html`
        <details class="settings-section sound-settings" onToggle=${event => {
            if (event.target === event.currentTarget) setOpen(event.currentTarget.open);
        }}>
            <summary class="settings-summary"><span class="section-title">Sound settings · ${device?.info?.name || deviceId}</span>
                <span class="settings-chevron" aria-hidden="true"></span></summary>
            ${content}
        </details>
    `;
}
