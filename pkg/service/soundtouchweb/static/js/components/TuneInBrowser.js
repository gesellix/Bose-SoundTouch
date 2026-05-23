import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

// BmxNavResponse has shape { bmx_sections: [{ name, items: [{ name, imageUrl, subtitle, _links }], _links }] }
// _links.bmx_navigate.href = "/v1/navigate/{encodedPath}" — strip prefix for API call
// _links.bmx_playback.href = station/track URL, type = "stationurl"|"tracklisturl"
// _links.bmx_next.href = "/v1/search/next?cursor={base64}" — load-more cursor

function navPath(item) {
    const href = item._links?.bmx_navigate?.href;
    return href ? href.replace(/^\/v1\/navigate\//, '') : null;
}

function playbackInfo(item) {
    const link = item._links?.bmx_playback;
    return link ? { location: link.href, type: link.type || 'stationurl' } : null;
}

function sectionCursor(section) {
    const href = section._links?.bmx_next?.href;
    if (!href) return null;
    return new URLSearchParams(href.split('?')[1] || '').get('cursor');
}

function toSections(data) {
    if (!data?.bmx_sections) return [];
    return data.bmx_sections.map(s => ({
        name: s.name,
        items: s.items || [],
        nextCursor: sectionCursor(s),
    }));
}

export function TuneInBrowser({ devices }) {
    const [sections, setSections] = useState([]);
    const [navStack, setNavStack] = useState([{ label: 'TuneIn', path: null }]);
    const [searchQuery, setSearchQuery] = useState('');
    const [loading, setLoading] = useState(false);
    const [pendingPlay, setPendingPlay] = useState(null);

    useEffect(() => { browse(null); }, []);

    async function browse(path) {
        setLoading(true);
        const resp = await api.tuneInBrowse(path);
        setLoading(false);
        if (resp.success) setSections(toSections(resp.data));
    }

    async function search(q) {
        if (!q.trim()) return;
        setLoading(true);
        const resp = await api.tuneInSearch(q);
        setLoading(false);
        if (resp.success) {
            setNavStack([{ label: 'TuneIn', path: null }, { label: `"${q}"`, path: null }]);
            setSections(toSections(resp.data));
        }
    }

    async function loadMore(section) {
        setLoading(true);
        const resp = await api.tuneInSearchNext(section.nextCursor);
        setLoading(false);
        if (!resp.success) return;
        const next = toSections(resp.data);
        const newItems = next.flatMap(s => s.items);
        const newCursor = next[0]?.nextCursor || null;
        setSections(prev => prev.map(s =>
            s.name === section.name
                ? { ...s, items: [...s.items, ...newItems], nextCursor: newCursor }
                : s
        ));
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

            ${navStack.length > 1 ? html`
                <nav class="breadcrumb">
                    ${navStack.map((entry, i) => html`
                        ${i > 0 ? html`<span class="breadcrumb-sep">›</span>` : null}
                        ${i < navStack.length - 1
                            ? html`<a class="breadcrumb-link" onClick=${() => navTo(i)}>${entry.label}</a>`
                            : html`<span class="breadcrumb-current">${entry.label}</span>`
                        }
                    `)}
                </nav>
            ` : null}

            ${loading ? html`<div class="loading-bar"></div>` : null}

            ${sections.map(section => html`
                <div>
                    ${section.name ? html`<h4 class="tunein-section-name">${section.name}</h4>` : null}
                    <ul class="tunein-list">
                        ${section.items.map((item, i) => {
                            const isNav = !!navPath(item);
                            const play = playbackInfo(item);
                            return html`
                                <li key=${item._links?.self?.href || i} class="tunein-item" onClick=${() => navigate(item)}>
                                    ${item.imageUrl ? html`<img class="tunein-thumb" src=${item.imageUrl} alt="" />` : null}
                                    <div class="tunein-item-info">
                                        <span class="tunein-item-name">${item.name}</span>
                                        ${item.subtitle ? html`<span class="tunein-item-desc">${item.subtitle}</span>` : null}
                                    </div>
                                    ${play ? html`
                                        <button
                                            class="tunein-play-btn"
                                            title="Play"
                                            onClick=${(e) => {
                                                e.stopPropagation();
                                                setPendingPlay({ ...play, name: item.name, image: item.imageUrl });
                                            }}
                                        >▶</button>
                                    ` : null}
                                    ${isNav ? html`<span class="tunein-item-arrow">›</span>` : null}
                                </li>
                            `;
                        })}
                    </ul>
                    ${section.nextCursor ? html`
                        <button class="btn-secondary tunein-load-more" onClick=${() => loadMore(section)}>
                            Load more
                        </button>
                    ` : null}
                </div>
            `)}

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
