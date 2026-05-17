import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

// BmxNavResponse has shape { bmx_sections: [{ name, items: [{ name, imageUrl, subtitle, _links }] }] }
// _links.bmx_navigate.href = "/v1/navigate/{encodedPath}" — strip prefix for API call
// _links.bmx_playback.href = station/track URL, type = "stationurl"|"tracklisturl"

function navPath(item) {
    const href = item._links?.bmx_navigate?.href;
    return href ? href.replace(/^\/v1\/navigate\//, '') : null;
}

function playbackInfo(item) {
    const link = item._links?.bmx_playback;
    return link ? { location: link.href, type: link.type || 'stationurl' } : null;
}

function flattenSections(data) {
    if (!data?.bmx_sections) return [];
    return data.bmx_sections.flatMap(section =>
        (section.items || []).map(item => ({ ...item, _sectionName: section.name }))
    );
}

export function TuneInBrowser({ devices }) {
    const [items, setItems] = useState([]);
    const [navStack, setNavStack] = useState([{ label: 'TuneIn', path: null }]);
    const [searchQuery, setSearchQuery] = useState('');
    const [loading, setLoading] = useState(false);
    const [pendingPlay, setPendingPlay] = useState(null);

    useEffect(() => { browse(null); }, []);

    async function browse(path) {
        setLoading(true);
        const resp = await api.tuneInBrowse(path);
        setLoading(false);
        if (resp.success) setItems(flattenSections(resp.data));
    }

    async function search(q) {
        if (!q.trim()) return;
        setLoading(true);
        const resp = await api.tuneInSearch(q);
        setLoading(false);
        if (resp.success) {
            setNavStack([{ label: 'TuneIn', path: null }, { label: `"${q}"`, path: null }]);
            setItems(flattenSections(resp.data));
        }
    }

    function navigate(item) {
        const path = navPath(item);
        const play = playbackInfo(item);

        if (path) {
            setNavStack(s => [...s, { label: item.name, path }]);
            browse(path);
        } else if (play) {
            setPendingPlay({ ...play, name: item.name, image: item.imageUrl });
        }
    }

    function navTo(index) {
        const stack = navStack.slice(0, index + 1);
        setNavStack(stack);
        browse(stack[stack.length - 1].path);
    }

    async function playOn(deviceId) {
        await api.tuneInPlay(deviceId, { location: pendingPlay.location, type: pendingPlay.type, name: pendingPlay.name });
        setPendingPlay(null);
    }

    const deviceEntries = Object.entries(devices);

    return html`
        <div class="tunein-browser">
            <div class="tunein-toolbar">
                <input
                    type="text"
                    class="tunein-search-input"
                    placeholder="Search stations, podcasts…"
                    value=${searchQuery}
                    onInput=${(e) => setSearchQuery(e.target.value)}
                    onKeyDown=${(e) => e.key === 'Enter' && search(searchQuery)}
                />
                <button class="btn-primary" onClick=${() => search(searchQuery)}>Search</button>
                <button class="btn-secondary" onClick=${() => {
                    setNavStack([{ label: 'TuneIn', path: null }]);
                    setSearchQuery('');
                    browse(null);
                }}>Browse</button>
            </div>

            ${navStack.length > 1 && html`
                <nav class="breadcrumb">
                    ${navStack.map((entry, i) => html`
                        ${i > 0 && html`<span class="breadcrumb-sep">›</span>`}
                        ${i < navStack.length - 1
                            ? html`<a class="breadcrumb-link" onClick=${() => navTo(i)}>${entry.label}</a>`
                            : html`<span class="breadcrumb-current">${entry.label}</span>`
                        }
                    `)}
                </nav>
            `}

            ${loading && html`<div class="loading-bar"></div>`}

            <ul class="tunein-list">
                ${items.map((item, i) => {
                    const isNav = !!navPath(item);
                    const play = playbackInfo(item);
                    // Uniform play affordance: any item with a playback
                    // link gets the same pill button (stops propagation
                    // so it doesn't trigger the row's navigate). The
                    // arrow span carries only the drill-in chevron;
                    // stations no longer reuse it for ▶, which kept the
                    // two affordances visually distinct.
                    return html`
                        <li key=${item._links?.self?.href || i} class="tunein-item" onClick=${() => navigate(item)}>
                            ${item.imageUrl && html`<img class="tunein-thumb" src=${item.imageUrl} alt="" />`}
                            <div class="tunein-item-info">
                                <span class="tunein-item-name">${item.name}</span>
                                ${item.subtitle && html`<span class="tunein-item-desc">${item.subtitle}</span>`}
                            </div>
                            ${play && html`
                                <button
                                    class="tunein-play-btn"
                                    title="Play"
                                    onClick=${(e) => {
                                        e.stopPropagation();
                                        setPendingPlay({ ...play, name: item.name, image: item.imageUrl });
                                    }}
                                >▶</button>
                            `}
                            ${isNav && html`<span class="tunein-item-arrow">›</span>`}
                        </li>
                    `;
                })}
            </ul>

            ${pendingPlay && html`
                <div class="overlay" onClick=${() => setPendingPlay(null)}>
                    <div class="device-picker" onClick=${(e) => e.stopPropagation()}>
                        <h3 class="picker-title">Play on device</h3>
                        <p class="picker-item-name">${pendingPlay.name}</p>
                        <div class="picker-devices">
                            ${deviceEntries.length === 0 && html`<p class="picker-no-devices">No devices found. Try discovering first.</p>`}
                            ${deviceEntries.map(([id, d]) => html`
                                <button class="picker-device-btn" onClick=${() => playOn(id)}>
                                    ${d.info?.name || id}
                                </button>
                            `)}
                        </div>
                        <button class="btn-secondary picker-cancel" onClick=${() => setPendingPlay(null)}>Cancel</button>
                    </div>
                </div>
            `}
        </div>
    `;
}