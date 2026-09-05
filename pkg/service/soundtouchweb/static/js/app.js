import { h, render } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import htm from 'htm';
import { DeviceList } from './components/DeviceList.js';
import { NowPlaying } from './components/NowPlaying.js';
import { Controls } from './components/Controls.js';
import { Presets } from './components/Presets.js';
import { Sources } from './components/Sources.js';
import { Zone } from './components/Zone.js';
import { StereoPair } from './components/StereoPair.js';
import { Recents } from './components/Recents.js';
import { TuneInBrowser } from './components/TuneInBrowser.js';
import { RadioBrowser } from './components/RadioBrowser.js';
import { Library } from './components/Library.js';
import { PlayURL } from './components/PlayURL.js';
import { TTS } from './components/TTS.js';
import { Announcements } from './components/Announcements.js';
import { api } from './api.js';
import { isSoundTouch10StereoPair } from './stereoPresentation.mjs';
import { removeDeviceAndRefresh } from './deviceRemoval.js';

const html = htm.bind(h);

function statusRevision(status) {
    const revision = status?.revision;
    return Number.isSafeInteger(revision) && revision >= 0 ? revision : null;
}

function statusEpoch(status) {
    const epoch = status?.epoch;
    return Number.isSafeInteger(epoch) ? epoch : null;
}

// The server advances DeviceStatus.revision on every projection, so a frame
// carrying a revision no newer than what we already hold is stale -- a slow
// `devices` snapshot overtaken by a `status_update` delta, or a REST refresh
// overtaken by either. A device we have never seen is always accepted.
//
// Revisions are only comparable within one epoch. A device id backed by a new
// DeviceConnection, or a restarted service, restarts its revisions at 0, and
// without the epoch check the browser would reject every later frame for that
// id and display a status frozen at whatever it last held.
function acceptsNewerStatus(current, incoming) {
    const currentEpoch = statusEpoch(current);
    const incomingEpoch = statusEpoch(incoming);
    if (currentEpoch !== null && incomingEpoch !== null && incomingEpoch !== currentEpoch) {
        return incomingEpoch > currentEpoch;
    }

    const currentRevision = statusRevision(current);
    const incomingRevision = statusRevision(incoming);
    if (currentRevision === null) return true;
    return incomingRevision !== null && incomingRevision > currentRevision;
}

export function mergeDevicesSnapshot(previous, snapshot) {
    return Object.fromEntries(Object.entries(snapshot || {}).map(([deviceId, incoming]) => {
        const current = Object.prototype.hasOwnProperty.call(previous, deviceId)
            ? previous[deviceId] : null;
        if (!current || acceptsNewerStatus(current.status, incoming?.status)) {
            return [deviceId, incoming];
        }
        // Keep the whole entry we already hold, not just its status. The
        // server derives stereoPair from the very status.Group this snapshot
        // lost the comparison on, so taking the incoming projection alongside
        // the newer status would describe a pair the newer status already
        // dissolved. The next snapshot carries a status we accept together
        // with a matching projection, and one is due within 5s (sooner in
        // practice: whatever produced the newer status also queued a
        // device-list broadcast).
        return [deviceId, current];
    }));
}

function replaceDevice(previous, deviceId, device) {
    return Object.fromEntries([
        ...Object.entries(previous),
        [deviceId, device],
    ]);
}

export function mergeStatusUpdate(previous, deviceId, status) {
    // Object.prototype.hasOwnProperty, not a plain previous[deviceId] truthy
    // check: a deviceId of "__proto__" or "constructor" would otherwise
    // resolve through the prototype chain to a truthy value and pass the
    // check despite not being a real, known device.
    if (!Object.prototype.hasOwnProperty.call(previous, deviceId) ||
        !acceptsNewerStatus(previous[deviceId]?.status, status)) {
        return previous;
    }
    return replaceDevice(previous, deviceId, {
        ...previous[deviceId],
        status,
    });
}

function DeviceDetail({ deviceId, devices, onBack, onDevicesChanged, notify, onRemove, onStatusReadback }) {
    const device = devices[deviceId];

    if (!device) {
        return html`
            <div class="page-header">
                <button class="back-btn" onClick=${onBack}>← Back</button>
            </div>
            <p>Device not found.</p>
        `;
    }

    return html`
        <div class="device-detail">
            <div class="page-header">
                <button class="back-btn" onClick=${onBack}>← Back</button>
                <button class="btn-icon" onClick=${() => api.power(deviceId)} title="Power">
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
                        <path d="M12 2v8" />
                        <path d="M18.36 6.64a9 9 0 1 1-12.73 0" />
                    </svg>
                </button>
            </div>
            <${NowPlaying} nowPlaying=${device.status?.nowPlaying} deviceId=${deviceId} presets=${device.status?.presets} />
            <${Controls} deviceId=${deviceId} status=${device.status} />
            <${Presets} deviceId=${deviceId} status=${device.status} />
            <${Sources}
                deviceId=${deviceId}
                status=${device.status}
                onStatusReadback=${status => onStatusReadback(deviceId, status)}
            />
            <${StereoPair}
                deviceId=${deviceId}
                device=${device}
                devices=${devices}
                onChanged=${onDevicesChanged}
                notify=${notify}
            />
            ${isSoundTouch10StereoPair(device) ? html`
                <aside class="stereo-pair-note" role="note" aria-label="Stereo pair limitation">
                    <strong>AirPlay unavailable while paired.</strong>
                    <span>SoundTouch 10 speakers cannot use AirPlay while paired. Unpair them to use AirPlay.</span>
                </aside>
            ` : null}
            <${Zone} deviceId=${deviceId} devices=${devices} />
            <${Recents} deviceId=${deviceId} />
            ${!device.stereoPair ? html`
                <div class="device-management-section">
                    <div class="section-title">Device management</div>
                    <button type="button"
                            class="btn-secondary device-remove-action"
                            aria-label=${`Remove ${device.info?.name || deviceId} from AfterTouch`}
                            onClick=${() => onRemove(deviceId)}>
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
                             stroke="currentColor" stroke-width="2" stroke-linecap="round"
                             stroke-linejoin="round" aria-hidden="true">
                            <path d="M3 6h18" />
                            <path d="M8 6V4h8v2" />
                            <path d="M19 6l-1 14H6L5 6" />
                            <path d="M10 11v5" />
                            <path d="M14 11v5" />
                        </svg>
                        <span>Remove from AfterTouch</span>
                    </button>
                </div>
            ` : null}
        </div>
    `;
}

function App() {
    const [devices, setDevices] = useState({});
    const [page, setPage] = useState('devices');
    const [selectedId, setSelectedId] = useState(null);
    const [toast, setToast] = useState(null);
    const [version, setVersion] = useState(null);
    const [isDiscovering, setIsDiscovering] = useState(false);

    const getPageTitle = () => {
        if (page === 'devices') return 'Devices';
        if (page === 'device') {
            const device = devices[selectedId];
            const name = device?.info?.name || selectedId || 'Device Detail';
            const ip = device?.info?.ip_address;
            if (ip) {
                return html`
                    <div class="title-with-subtitle">
                        <span class="main-title">${name}</span>
                        <span class="sub-title">${ip}</span>
                    </div>
                `;
            }
            return name;
        }
        if (page === 'tunein') return 'TuneIn';
        if (page === 'radiobrowser') return 'RadioBrowser';
        if (page === 'library') return 'Library';
        if (page === 'playurl') return 'Play URL';
        if (page === 'tts') return 'TTS';
        return 'AfterTouch';
    };

    useEffect(() => {
        fetch('/api/control/version')
            .then(res => res.json())
            .then(resp => {
                if (resp.success) {
                    setVersion(resp.data);
                }
            })
            .catch(err => console.error('Failed to fetch version:', err));

        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(`${protocol}//${location.host}/api/control/ws`);
        let reconnectTimer;

        ws.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            if (msg.type === 'devices') {
                setDevices(previous => mergeDevicesSnapshot(previous, msg.data));
            } else if (msg.type === 'discovery_status') {
                if (msg.data?.isDiscovering !== undefined) {
                    setIsDiscovering(msg.data.isDiscovering);
                } else if (msg.data?.status === 'starting') {
                    setIsDiscovering(true);
                } else if (msg.data?.status === 'completed') {
                    setIsDiscovering(false);
                }

                if (msg.data?.status === 'completed') {
                    showToast(`Found ${msg.data.deviceCount} device(s)`);
                }
            } else if (msg.type === 'status_update' && msg.deviceId) {
                setDevices(previous => mergeStatusUpdate(previous, msg.deviceId, msg.data));
            }
        };

        ws.onclose = () => {
            reconnectTimer = setTimeout(() => location.reload(), 5000);
        };

        return () => {
            clearTimeout(reconnectTimer);
            ws.close();
        };
    }, []);

    useEffect(() => {
        if (selectedId && !devices[selectedId]) {
            setSelectedId(null);
            if (page === 'device') setPage('devices');
        }
    }, [devices, selectedId, page]);

    function showToast(msg) {
        setToast(null);
        setTimeout(() => setToast(msg), 10);
        setTimeout(() => setToast(null), 3000);
    }

    const navigate = useCallback((p, id = null) => {
        setPage(p);
        setSelectedId(id);
    }, []);

    async function discover() {
        showToast('Discovering devices…');
        await api.discover();
    }

    async function refreshDevices() {
        const resp = await api.devices();
        if (!resp?.success) throw new Error(resp?.error || 'Failed to refresh devices');
        // Ordered like the WebSocket frames: a slow REST snapshot must not
        // clobber a newer status that arrived over the socket meanwhile.
        setDevices(previous => mergeDevicesSnapshot(previous, resp.data));
    }

    function mergeDeviceReadback(deviceId, status) {
        setDevices(previous => mergeStatusUpdate(previous, deviceId, status));
    }

    async function removeDevice(id) {
        const name = devices[id]?.info?.name || id;
        if (!confirm(`Remove "${name}" from AfterTouch?\n\nThis does not reset the speaker. A device still online may reappear after the next discovery scan.`)) {
            return;
        }
        await removeDeviceAndRefresh({
            id,
            name,
            remove: api.removeDevice,
            refresh: refreshDevices,
            showDeviceList: () => navigate('devices'),
            notify: showToast,
        });
    }

    return html`
        <div class="app">
            <nav class="navbar">
                <a class="brand" href="/?chooser" title="AfterTouch home">
                    <img src="/app/static/img/logo.svg" alt="AfterTouch" class="nav-logo" />
                    <div class="brand-text">
                        <span class="brand-name">AfterTouch</span>
                        <span class="brand-subtitle">Bose SoundTouch Toolkit</span>
                    </div>
                </a>
                <div class="page-title">${getPageTitle()}</div>
                <div class="nav-links">
                    <a href="#" class="${page === 'devices' || page === 'device' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('devices'); }}
                        title="Devices"
                    >
                        <img src="/app/static/img/speaker-mono.svg" alt="Devices" class="nav-device-icon" />
                    </a>
                    <a href="#" class="${page === 'tunein' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('tunein'); }}
                        title="TuneIn"
                    >
                        <img src="/app/static/img/tunein-mono.svg" alt="TuneIn" class="nav-tunein-icon" />
                    </a>
                    <a href="#" class="${page === 'radiobrowser' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('radiobrowser'); }}
                        title="RadioBrowser"
                    >
                        <img src="/app/static/img/radiobrowser-mono.svg" alt="RadioBrowser" class="nav-rb-icon" />
                    </a>
                    <a href="#" class="${page === 'playurl' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('playurl'); }}
                        title="Play URL"
                    >
                        <img src="/app/static/img/link-mono.svg" alt="Play URL" class="nav-url-icon" />
                    </a>
                    <a href="#" class="${page === 'library' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('library'); }}
                        title="Library"
                        style="font-size:.75rem;font-weight:600;letter-spacing:.02em"
                    >Lib</a>
                    <a href="#" class="${page === 'tts' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('tts'); }}
                        title="TTS"
                    >
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                            <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
                            <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
                            <path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>
                        </svg>
                    </a>
                    <span class="nav-separator">|</span>
                    <button class="btn-icon" onClick=${discover} title="Discover">
                        <img src="/app/static/img/knob-mono.svg" alt="Discover" class="nav-discover-icon ${isDiscovering ? 'buzzing' : ''}" />
                    </button>
                    <a href="https://gesellix.github.io/Bose-SoundTouch/" target="_blank" rel="noopener" title="Documentation">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                            <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/>
                            <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/>
                        </svg>
                    </a>
                </div>
            </nav>

            <${Announcements} />

            <main class="main-content">
                ${page === 'devices' ? html`
                    <${DeviceList}
                        key="device-list"
                        devices=${devices}
                        isDiscovering=${isDiscovering}
                        onSelect=${(id) => navigate('device', id)}
                        onDiscover=${discover}
                    />
                ` : page === 'device' ? html`
                    <${DeviceDetail}
                        key="device-detail"
                        deviceId=${selectedId}
                        devices=${devices}
                        onBack=${() => navigate('devices')}
                        onDevicesChanged=${refreshDevices}
                        notify=${showToast}
                        onRemove=${removeDevice}
                        onStatusReadback=${mergeDeviceReadback}
                    />
                ` : page === 'tunein' ? html`
                    <${TuneInBrowser} key="tunein-browser" devices=${devices} />
                ` : page === 'radiobrowser' ? html`
                    <${RadioBrowser} key="radiobrowser-browser" devices=${devices} />
                ` : page === 'playurl' ? html`
                    <${PlayURL} key="play-url" devices=${devices} serverServiceUrl=${version?.service_url || ''} />
                ` : page === 'tts' ? html`
                    <${TTS} key="tts" devices=${devices} serverServiceUrl=${version?.service_url || ''} />
                ` : page === 'library' ? html`
                    <${Library} key="library" devices=${devices} />
                ` : null}
            </main>

                ${version ? html`
                    <footer id="footer" key="footer">
                        <span>
                            AfterTouch <a href="${version.release_url || version.repo_url}" target="_blank">${version.version}</a>
                            ${version.commit && version.commit !== 'unknown' ? html`
                                ${' ('}<a href="${version.commit_url}" target="_blank">${version.commit.substring(0, 7)}</a>${')'}
                            ` : null}
                            ${version.date && version.date !== 'unknown' ? html` • ${version.date}` : null}
                        </span>
                    </footer>
                ` : null}

            ${toast ? html`<div class="toast" role="status" aria-live="polite"
                                aria-atomic="true" key="toast">${toast}</div>` : null}
        </div>
    `;
}

const appRoot = document.getElementById('app');
if (appRoot) render(html`<${App} />`, appRoot);
