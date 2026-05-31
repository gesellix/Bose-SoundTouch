import { h } from 'preact';
import { useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

// TTS is a "source" view (like PlayURL / TuneIn / RadioBrowser): enter text,
// pick a device, and the AfterTouch service synthesizes and plays it. Synthesis
// and credentials live in the service; this just collects text and a target.
export function TTS({ devices }) {
    const [text, setText] = useState('');
    const [pendingSpeak, setPendingSpeak] = useState(null);
    const [status, setStatus] = useState(null);

    function startSpeak() {
        const trimmed = text.trim();
        if (!trimmed) return;
        setStatus(null);
        setPendingSpeak({ text: trimmed });
    }

    async function speakOn(deviceId) {
        const item = pendingSpeak;
        setPendingSpeak(null);
        setStatus('Speaking…');
        try {
            const resp = await api.speak(deviceId, item.text);
            setStatus(resp.success ? 'Speaking' : 'Error: ' + (resp.error || 'Unknown error'));
        } catch (e) {
            setStatus('Error: ' + e.message);
        }
    }

    const deviceEntries = Object.entries(devices);

    return html`
        <div class="tunein-browser">
            <div class="tunein-toolbar">
                <input
                    type="text"
                    class="tunein-search-input"
                    placeholder="Say something…"
                    value=${text}
                    onInput=${(e) => setText(e.target.value)}
                    onKeyDown=${(e) => e.key === 'Enter' && startSpeak()}
                />
                <button class="btn-primary" onClick=${startSpeak} disabled=${!text.trim()}>🔊 Speak</button>
            </div>
            <div class="track-meta" style="margin-top:.4rem">
                Uses the AfterTouch service's configured TTS provider. Requires soundtouch-web to be started with --service-url.
            </div>
            ${status && html`<div class="track-meta" style="margin-top:.6rem">${status}</div>`}

            ${pendingSpeak ? html`
                <div class="overlay" onClick=${() => setPendingSpeak(null)}>
                    <div class="device-picker" onClick=${(e) => e.stopPropagation()}>
                        <h3 class="picker-title">Speak on device</h3>
                        <p class="picker-item-name">${pendingSpeak.text}</p>
                        <div class="picker-devices">
                            ${deviceEntries.length === 0 ? html`<p class="picker-no-devices">No devices found. Try discovering first.</p>` : null}
                            ${deviceEntries.map(([id, d]) => html`
                                <button class="picker-device-btn" key=${id} onClick=${() => speakOn(id)}>
                                    <div class="picker-device-info">
                                        <span class="picker-device-name">${d.info?.name || id}</span>
                                        <span class="picker-device-ip">${d.info?.ip_address || ''}</span>
                                    </div>
                                </button>
                            `)}
                        </div>
                        <button class="btn-secondary picker-cancel" onClick=${() => setPendingSpeak(null)}>Cancel</button>
                    </div>
                </div>
            ` : null}
        </div>
    `;
}