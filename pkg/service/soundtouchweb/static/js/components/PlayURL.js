import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

const LS_KEY = 'aftertouch_service_url';

export function PlayURL({ devices, serverServiceUrl }) {
    const [url, setUrl] = useState('');
    const [name, setName] = useState('');
    const [serviceUrl, setServiceUrl] = useState(() => localStorage.getItem(LS_KEY) || '');
    const [pendingPlay, setPendingPlay] = useState(null);
    const [status, setStatus] = useState(null);

    useEffect(() => {
        if (serverServiceUrl && !localStorage.getItem(LS_KEY)) {
            setServiceUrl(serverServiceUrl);
        }
    }, [serverServiceUrl]);

    function onServiceUrlChange(val) {
        setServiceUrl(val);
        if (val) {
            localStorage.setItem(LS_KEY, val);
        } else {
            localStorage.removeItem(LS_KEY);
        }
    }

    function startPlay() {
        const trimmedUrl = url.trim();
        if (!trimmedUrl) return;
        setStatus(null);
        setPendingPlay({ url: trimmedUrl, name: name.trim() || trimmedUrl });
    }

    async function playOn(deviceId) {
        const item = pendingPlay;
        setPendingPlay(null);
        setStatus('Playing…');
        try {
            // When configured server-side, that value wins; send it so a stale
            // localStorage override never matters.
            const effectiveServiceUrl = serverServiceUrl || serviceUrl.trim();
            const resp = await api.playURL(deviceId, item.url, item.name, '', effectiveServiceUrl);
            setStatus(resp.success ? 'Playing — use ★ on the device page to save as preset' : 'Error: ' + (resp.error || 'Unknown error'));
        } catch (e) {
            setStatus('Error: ' + e.message);
        }
    }

    const deviceEntries = Object.entries(devices);

    return html`
        <div class="tunein-browser">
            <div class="tunein-toolbar">
                <input
                    type="url"
                    class="tunein-search-input"
                    placeholder="Stream URL (http://…)"
                    value=${url}
                    onInput=${(e) => setUrl(e.target.value)}
                    onKeyDown=${(e) => e.key === 'Enter' && startPlay()}
                />
                <input
                    type="text"
                    class="tunein-search-input"
                    placeholder="Name (optional)"
                    value=${name}
                    style="max-width:160px"
                    onInput=${(e) => setName(e.target.value)}
                    onKeyDown=${(e) => e.key === 'Enter' && startPlay()}
                />
                <button class="btn-primary" onClick=${startPlay} disabled=${!url.trim()}>▶ Play</button>
            </div>
            <div class="tunein-toolbar" style="margin-top:.4rem">
                <input
                    type="url"
                    class="tunein-search-input"
                    placeholder="AfterTouch URL (https://…)"
                    value=${serverServiceUrl || serviceUrl}
                    onInput=${(e) => onServiceUrlChange(e.target.value)}
                    readonly=${!!serverServiceUrl}
                    title="AfterTouch service base URL — required for LOCAL_INTERNET_RADIO playback and preset save"
                />
            </div>
            ${serverServiceUrl
                ? html`<div class="track-meta" style="margin-top:.2rem; opacity:.85">Configured server-side (soundtouch-web --service-url); edits here would be ignored.</div>`
                : null}
            ${status && html`<div class="track-meta" style="margin-top:.6rem">${status}</div>`}

            ${pendingPlay ? html`
                <div class="overlay" onClick=${() => setPendingPlay(null)}>
                    <div class="device-picker" onClick=${(e) => e.stopPropagation()}>
                        <h3 class="picker-title">Play on device</h3>
                        <p class="picker-item-name">${pendingPlay.name}</p>
                        <div class="picker-devices">
                            ${deviceEntries.length === 0 ? html`<p class="picker-no-devices">No devices found. Try discovering first.</p>` : null}
                            ${deviceEntries.map(([id, d]) => html`
                                <button class="picker-device-btn" key=${id} onClick=${() => playOn(id)}>
                                    <div class="picker-device-info">
                                        <span class="picker-device-name">${d.info?.name || id}</span>
                                        <span class="picker-device-ip">${d.info?.ip_address || ''}</span>
                                    </div>
                                </button>
                            `)}
                        </div>
                        <button class="btn-secondary picker-cancel" onClick=${() => setPendingPlay(null)}>Cancel</button>
                    </div>
                </div>
            ` : null}
        </div>
    `;
}
