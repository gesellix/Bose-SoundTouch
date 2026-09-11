import { h } from 'preact';
import { useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

function flattenSections(data) {
    if (!data?.bmx_sections) return [];
    return data.bmx_sections.flatMap(section =>
        (section.items || []).map(item => ({ ...item, _sectionName: section.name }))
    );
}

export function RadioBrowser({
    devices,
    onPlaybackRequest,
    playbackBusy = false,
    commandReadbackDelays,
}) {
    const [items, setItems] = useState([]);
    const [searchQuery, setSearchQuery] = useState('');
    const [loading, setLoading] = useState(false);
    const [pendingPlay, setPendingPlay] = useState(null);

    async function search(q) {
        if (!q.trim()) return;
        setLoading(true);
        const resp = await api.radioBrowserSearch(q);
        setLoading(false);
        if (resp.success) {
            setItems(flattenSections(resp.data));
        }
    }

    function playOn(deviceId) {
        const item = pendingPlay;
        if (!item) return;
        const accepted = onPlaybackRequest?.({
            deviceId,
            action: 'radiobrowser',
            readbackDelays: commandReadbackDelays,
            invoke: () => api.radioBrowserPlayChecked(deviceId, {
                location: item.location,
                type: item.type,
                name: item.name,
                // The speaker keeps this on the ContentItem, so presets and
                // recents saved from here show the station logo.
                containerArt: pendingPlay.image,
            }),
            expected: {
                source: 'RADIO_BROWSER',
                location: item.location,
                itemName: item.name,
            },
        });
        if (accepted !== false) setPendingPlay(null);
    }

    const deviceEntries = Object.entries(devices);

    return html`
        <div class="tunein-browser">
            <div class="tunein-toolbar">
                <input
                    type="text"
                    class="tunein-search-input"
                    placeholder="Search RadioBrowser stations…"
                    value=${searchQuery}
                    onInput=${(e) => setSearchQuery(e.target.value)}
                    onKeyDown=${(e) => e.key === 'Enter' && search(searchQuery)}
                />
                <button class="btn-primary" onClick=${() => search(searchQuery)}>Search</button>
            </div>

            ${loading ? html`<div class="loading-bar"></div>` : null}

            <ul class="tunein-list">
                ${items.length === 0 && !loading ? html`<li class="tunein-item" key="empty">No results yet. Try searching for a station.</li>` : null}
                ${items.map((item, i) => {
                    const play = item._links?.bmx_playback;
                    return html`
                        <li key=${item.stationuuid || i} class="tunein-item">
                            ${item.imageUrl ? html`<img class="tunein-thumb" src=${item.imageUrl} alt="" />` : null}
                            <div class="tunein-item-info">
                                <span class="tunein-item-name">${item.name}</span>
                                ${item.subtitle ? html`<span class="tunein-item-desc">${item.subtitle}</span>` : null}
                            </div>
                            ${play ? html`
                                <button
                                    class="tunein-play-btn"
                                    title="Play"
                                    disabled=${playbackBusy}
                                    onClick=${() => {
                                        setPendingPlay({ location: play.href, type: play.type, name: item.name, image: item.imageUrl });
                                    }}
                                >▶</button>
                            ` : null}
                        </li>
                    `;
                })}
            </ul>

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
