import { h } from 'preact';
import { useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

// The stream is played through a relative Orion location ("/station?data=..."),
// which the speaker resolves against the service address in its BMX registry.
// So Play URL needs no AfterTouch URL of its own, and a preset saved from it
// keeps working when AfterTouch moves (issue 769).
export function PlayURL({
    devices,
    onPlaybackRequest,
    playbackBusy = false,
    commandReadbackDelays,
}) {
    const [url, setUrl] = useState('');
    const [name, setName] = useState('');
    const [pendingPlay, setPendingPlay] = useState(null);

    function startPlay() {
        const trimmedUrl = url.trim();
        if (!trimmedUrl) return;
        setPendingPlay({ url: trimmedUrl, name: name.trim() || trimmedUrl });
    }

    function playOn(deviceId) {
        const item = pendingPlay;
        if (!item) return;
        const accepted = onPlaybackRequest?.({
            deviceId,
            action: 'url',
            readbackDelays: commandReadbackDelays,
            invoke: () => api.playURLChecked(deviceId, item.url, item.name, ''),
            expected: {
                source: 'LOCAL_INTERNET_RADIO',
                itemName: item.name,
            },
            expectedFromResponse: response => {
                const location = response?.data?.location;
                if (!location) return null;
                return {
                    source: response.data.source || 'LOCAL_INTERNET_RADIO',
                    location,
                    itemName: response.data.itemName || item.name,
                };
            },
        });
        if (accepted !== false) setPendingPlay(null);
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
                <button class="btn-primary" onClick=${startPlay} disabled=${!url.trim() || playbackBusy}>▶ Play</button>
            </div>
            ${pendingPlay ? html`
                <div class="overlay" onClick=${() => setPendingPlay(null)}>
                    <div class="device-picker" onClick=${(e) => e.stopPropagation()}>
                        <h3 class="picker-title">Play on device</h3>
                        <p class="picker-item-name">${pendingPlay.name}</p>
                        <div class="picker-devices">
                            ${deviceEntries.length === 0 ? html`<p class="picker-no-devices">No devices found. Try discovering first.</p>` : null}
                            ${deviceEntries.map(([id, d]) => html`
                                <button
                                    class="picker-device-btn"
                                    key=${id}
                                    disabled=${playbackBusy}
                                    onClick=${() => playOn(id)}
                                >
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
