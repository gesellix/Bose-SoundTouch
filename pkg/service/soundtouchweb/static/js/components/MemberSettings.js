import { h } from 'preact';
import { useState } from 'preact/hooks';
import htm from 'htm';
import { physicalSettingsTargets } from '../soundSettingsPresentation.mjs';
import { Settings } from './Settings.js';
import { SoundSettings } from './SoundSettings.js';

const html = htm.bind(h);

export function MemberSettings({ deviceId, device, nested = true }) {
    const [open, setOpen] = useState(false);
    const targets = physicalSettingsTargets(deviceId, device);
    if (!targets.length) return null;
    const content = html`
        <${SoundSettings} deviceId=${deviceId} device=${device} nested=${nested} active=${open} />
        ${targets.map(target => html`<${Settings} key=${`${target.controlId}:${target.physicalId}`}
            deviceId=${target.controlId} targetIdentity=${target.physicalId}
            targetName=${target.name} targetRole=${target.role} nested=${nested} />`)}
    `;
    return nested ? html`<details class="member-settings settings-section"
        onToggle=${event => { if (event.target === event.currentTarget) setOpen(event.currentTarget.open); }}>
        <summary class="settings-summary"><span class="section-title">Settings · ${device?.info?.name || deviceId}</span>
            <span class="settings-chevron" aria-hidden="true"></span></summary>
        <div class="member-settings-content">${content}</div>
    </details>` : content;
}
