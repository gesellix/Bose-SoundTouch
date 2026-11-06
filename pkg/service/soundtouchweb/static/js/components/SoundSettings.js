import { h } from 'preact';
import { useRef } from 'preact/hooks';
import htm from 'htm';
import { bassControl, pairBassControl, balanceControl, physicalSettingsTargets } from '../soundSettingsPresentation.mjs';
import { SteppedSoundControl } from './SteppedSoundControl.js';

const html = htm.bind(h);

export function SoundSettings({ deviceId, device, nested = false }) {
    const lastPresentation = useRef({ key: '', bass: {}, pairBass: null, balance: null });
    const targets = physicalSettingsTargets(deviceId, device);
    const identityPending = Boolean(device?.settingsIdentityPending || device?.volumeIdentityPending);
    const ownerKey = JSON.stringify([deviceId, device?.stereoPair?.id || '', device?.stereoPair?.masterDeviceId || '',
        targets.map(target => [target.controlId, target.physicalId, target.role, target.status?.epoch])]);
    if (lastPresentation.current.key !== ownerKey) lastPresentation.current = { key: ownerKey, bass: {}, pairBass: null, balance: null };
    const bassTargets = targets.map(target => {
        const current = identityPending ? null : bassControl(target);
        const presentationKey = `${target.controlId}:${target.physicalId}`;
        if (current) lastPresentation.current.bass[presentationKey] = current;
        return { target, control: current || lastPresentation.current.bass[presentationKey], valid: Boolean(current) };
    });
    const currentBalance = identityPending ? null : balanceControl(deviceId, device);
    if (currentBalance) lastPresentation.current.balance = currentBalance;
    const balance = currentBalance || lastPresentation.current.balance;
    const paired = Boolean(device?.stereoPair);
    const currentPairBass = paired && bassTargets.every(entry => entry.valid)
        ? pairBassControl(deviceId, { ...device, settingsTargets: bassTargets.map(entry => entry.target) }) : null;
    if (currentPairBass) lastPresentation.current.pairBass = currentPairBass;
    const pairBass = currentPairBass || lastPresentation.current.pairBass;
    const content = html`
        <div class="settings-content sound-settings-content">
            ${!paired ? bassTargets.filter(entry => entry.control).map(({ target, control, valid }) => html`<${SteppedSoundControl}
                key=${`bass:${target.controlId}:${target.physicalId}`} kind="bass"
                target=${target} control=${control} ownerKey=${JSON.stringify([ownerKey, identityPending])} available=${valid} />`) : null}
            ${paired && pairBass ? html`<${SteppedSoundControl}
                key=${`pair-bass:${pairBass.controlId}:${pairBass.pairId}`} kind="bass"
                target=${pairBass} control=${pairBass} ownerKey=${JSON.stringify([ownerKey, identityPending])} available=${Boolean(currentPairBass)} />` : null}
            ${paired && !currentPairBass && (pairBass || bassTargets.some(entry => entry.control))
                ? html`<div class="sound-control-readback">Pair bass requires both speakers to report matching settings and a confirmed stereo pair.</div>` : null}
            ${currentPairBass?.observationConfirmed === false
                ? html`<div class="sound-control-readback">The speakers currently report different bass values.</div>` : null}
            ${balance ? html`<${SteppedSoundControl} key=${`balance:${balance.controlId}:${balance.pairId}`}
                kind="balance" target=${{ ...balance, status: device.status }} control=${balance} ownerKey=${JSON.stringify([ownerKey, identityPending])}
                available=${Boolean(currentBalance)} />` : null}
            ${!balance && !bassTargets.some(entry => entry.control)
                ? html`<div class="sound-control-readback">No supported sound controls reported.</div>` : null}
        </div>
    `;
    return nested ? html`<section class="sound-settings"><div class="section-title">Sound</div>${content}</section>` : html`
        <details class="settings-section sound-settings">
            <summary class="settings-summary"><span class="section-title">Sound settings · ${device?.info?.name || deviceId}</span>
                <span class="settings-chevron" aria-hidden="true"></span></summary>
            ${content}
        </details>
    `;
}
