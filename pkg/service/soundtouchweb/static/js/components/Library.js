import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

export function Library({ devices }) {
    const deviceEntries = Object.entries(devices);
    const firstDeviceId = deviceEntries.length > 0 ? deviceEntries[0][0] : null;

    const [deviceId, setDeviceId] = useState(firstDeviceId);
    const [servers, setServers] = useState([]);
    const [discovered, setDiscovered] = useState([]);
    const [server, setServer] = useState(null);       // { udn, name, account }
    const [navStack, setNavStack] = useState([]);     // [{ label, location, type }]
    const [entries, setEntries] = useState([]);
    const [loading, setLoading] = useState(false);
    const [finding, setFinding] = useState(false);
    const [playingName, setPlayingName] = useState(null);

    // Sync deviceId when devices prop first arrives or changes enough to
    // invalidate the current selection.
    useEffect(() => {
        const entries = Object.entries(devices);
        if (!deviceId && entries.length > 0) {
            setDeviceId(entries[0][0]);
        }
    }, [devices]);

    // Reload registered servers whenever deviceId changes.
    useEffect(() => {
        if (!deviceId) return;
        setServer(null);
        setNavStack([]);
        setEntries([]);
        loadServers(deviceId);
    }, [deviceId]);

    async function loadServers(id) {
        setLoading(true);
        const resp = await api.libraryServers(id);
        setLoading(false);
        if (resp.success) setServers(resp.data || []);
    }

    async function discover() {
        setLoading(true);
        const resp = await api.libraryDiscover(6);
        setLoading(false);
        if (resp.success) setDiscovered(resp.data || []);
    }

    async function addServer(srv) {
        await api.libraryAddServer(deviceId, { udn: srv.udn, name: srv.name });
        await loadServers(deviceId);
        setFinding(false);
    }

    async function removeServer(srv) {
        await api.libraryRemoveServer(deviceId, `${srv.udn}/0`);
        if (server && server.udn === srv.udn) {
            setServer(null);
            setNavStack([]);
            setEntries([]);
        }
        await loadServers(deviceId);
    }

    async function openServer(srv) {
        const account = `${srv.udn}/0`;
        setServer({ udn: srv.udn, name: srv.name, account });
        const root = { label: srv.name, location: '', type: '' };
        setNavStack([root]);
        await browseLevel(account, '', '');
    }

    async function browseLevel(account, location, type) {
        setLoading(true);
        const resp = await api.libraryBrowse(deviceId, { account, location, type });
        setLoading(false);
        if (resp.success) setEntries(resp.data?.entries || []);
    }

    async function browseEntry(entry) {
        const newFrame = { label: entry.name, location: entry.location, type: entry.type };
        setNavStack(s => [...s, newFrame]);
        await browseLevel(server.account, entry.location, entry.type);
    }

    async function navTo(index) {
        const stack = navStack.slice(0, index + 1);
        setNavStack(stack);
        const frame = stack[stack.length - 1];
        await browseLevel(server.account, frame.location, frame.type);
    }

    async function playEntry(entry) {
        // Pass the entry's own type so a folder selects as a container ("dir")
        // rather than a single track — that lets the speaker queue the folder so
        // next/previous and auto-advance work, instead of stopping after one item.
        await api.libraryPlay(deviceId, {
            account: server.account,
            location: entry.location,
            type: entry.type || 'track',
            name: entry.name,
        });
        setPlayingName(entry.name);
        setTimeout(() => setPlayingName(null), 3000);
    }

    function toggleFinding() {
        const next = !finding;
        setFinding(next);
        if (next && discovered.length === 0) discover();
    }

    const registeredUdns = new Set(servers.map(s => s.udn));

    return html`
        <div class="tunein-browser">

            ${deviceEntries.length === 0 ? html`
                <p class="tunein-item-desc" style="padding:.75rem 0">
                    No devices found. Discover devices first.
                </p>
            ` : html`
                <div class="tunein-toolbar" style="flex-wrap:wrap;gap:.5rem;align-items:center">
                    ${deviceEntries.length > 1 ? html`
                        <select
                            value=${deviceId}
                            onChange=${(e) => setDeviceId(e.target.value)}
                            style="padding:.4rem .6rem;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);color:var(--text);font:inherit;font-size:.875rem"
                        >
                            ${deviceEntries.map(([id, d]) => html`
                                <option key=${id} value=${id}>${d.info?.name || id}</option>
                            `)}
                        </select>
                    ` : html`
                        <span style="font-size:.875rem;color:var(--text-dim)">
                            ${devices[deviceId]?.info?.name || deviceId}
                        </span>
                    `}
                    <button
                        class=${finding ? 'btn-primary' : 'btn-secondary'}
                        onClick=${toggleFinding}
                    >
                        ${finding ? 'Hide' : 'Find servers'}
                    </button>
                    <span style="font-size:.7rem;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--text-dim);padding:.2rem .4rem;border:1px solid var(--border);border-radius:4px">
                        BETA
                    </span>
                </div>
            `}

            ${loading ? html`<div class="loading-bar"></div>` : null}

            ${finding ? html`
                <div style="background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:1rem;margin-bottom:1rem">
                    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:.75rem">
                        <span style="font-size:.8rem;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--text-dim)">
                            LAN media servers
                        </span>
                        <button class="btn-secondary" style="font-size:.8rem;padding:.3rem .6rem" onClick=${discover}>
                            Rescan
                        </button>
                    </div>
                    ${discovered.length === 0 ? html`
                        <p style="font-size:.875rem;color:var(--text-dim)">No servers found yet. Click Rescan to search.</p>
                    ` : html`
                        <ul class="tunein-list">
                            ${discovered.map((srv, i) => {
                                const already = registeredUdns.has(srv.udn);
                                return html`
                                    <li key=${srv.udn || i} class="tunein-item">
                                        <div class="tunein-item-info">
                                            <span class="tunein-item-name">${srv.name}</span>
                                            ${srv.manufacturer ? html`<span class="tunein-item-desc">${srv.manufacturer}${srv.model ? ` — ${srv.model}` : ''}</span>` : null}
                                        </div>
                                        ${already
                                            ? html`<span style="font-size:.8rem;color:var(--text-dim)">Added</span>`
                                            : html`<button class="btn-primary" style="font-size:.8rem;padding:.3rem .7rem;white-space:nowrap" onClick=${() => addServer(srv)}>Add</button>`
                                        }
                                    </li>
                                `;
                            })}
                        </ul>
                    `}
                </div>
            ` : null}

            ${!server && servers.length === 0 && !loading ? html`
                <p style="font-size:.875rem;color:var(--text-dim);padding:.5rem 0">
                    No media servers registered on this device. Use "Find servers" to add one.
                </p>
            ` : null}

            ${servers.length > 0 && !server ? html`
                <div style="margin-bottom:1rem">
                    <p class="section-title" style="margin-bottom:.5rem">Media servers</p>
                    <ul class="tunein-list">
                        ${servers.map((srv, i) => html`
                            <li key=${srv.udn || i} class="tunein-item" onClick=${() => openServer(srv)}>
                                <div class="tunein-item-info">
                                    <span class="tunein-item-name">${srv.name}</span>
                                    ${!srv.ready ? html`<span class="tunein-item-desc">(connecting…)</span>` : null}
                                </div>
                                <button
                                    class="btn-icon"
                                    title="Remove"
                                    style="color:var(--text-dim);font-size:.85rem;width:28px;height:28px"
                                    onClick=${(e) => { e.stopPropagation(); removeServer(srv); }}
                                >✕</button>
                                <span class="tunein-item-arrow">›</span>
                            </li>
                        `)}
                    </ul>
                </div>
            ` : null}

            ${server ? html`
                <div>
                    ${navStack.length > 0 ? html`
                        <nav class="breadcrumb">
                            ${navStack.map((frame, i) => html`
                                ${i > 0 ? html`<span class="breadcrumb-sep">›</span>` : null}
                                ${i < navStack.length - 1
                                    ? html`<a class="breadcrumb-link" onClick=${() => navTo(i)}>${frame.label}</a>`
                                    : html`<span class="breadcrumb-current">${frame.label}</span>`
                                }
                            `)}
                        </nav>
                    ` : null}

                    ${playingName ? html`
                        <div style="font-size:.875rem;color:var(--text-dim);margin-bottom:.5rem">
                            Playing: <strong>${playingName}</strong>
                        </div>
                    ` : null}

                    ${entries.length === 0 && !loading ? html`
                        <p style="font-size:.875rem;color:var(--text-dim);padding:.5rem 0">No items found.</p>
                    ` : null}

                    <ul class="tunein-list">
                        ${entries.map((entry, i) => html`
                            <li
                                key=${entry.location || i}
                                class="tunein-item"
                                onClick=${() => entry.isDir ? browseEntry(entry) : null}
                                style=${!entry.isDir ? 'cursor:default' : ''}
                            >
                                <div class="tunein-item-info">
                                    <span class="tunein-item-name">${entry.name}</span>
                                    ${entry.type ? html`<span class="tunein-item-desc">${entry.type}</span>` : null}
                                </div>
                                ${entry.playable || entry.isDir ? html`
                                    <button
                                        class="tunein-play-btn"
                                        title="${entry.isDir ? 'Play folder' : 'Play'} on ${devices[deviceId]?.info?.name || deviceId}"
                                        onClick=${(e) => { e.stopPropagation(); playEntry(entry); }}
                                    >▶</button>
                                ` : null}
                                ${entry.isDir ? html`<span class="tunein-item-arrow">›</span>` : null}
                            </li>
                        `)}
                    </ul>
                </div>
            ` : null}

        </div>
    `;
}
