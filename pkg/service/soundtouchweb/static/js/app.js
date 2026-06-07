import { h, render } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import htm from 'htm';
import { DeviceList } from './components/DeviceList.js';
import { NowPlaying } from './components/NowPlaying.js';
import { Controls } from './components/Controls.js';
import { Presets } from './components/Presets.js';
import { Sources } from './components/Sources.js';
import { Zone } from './components/Zone.js';
import { Recents } from './components/Recents.js';
import { TuneInBrowser } from './components/TuneInBrowser.js';
import { RadioBrowser } from './components/RadioBrowser.js';
import { PlayURL } from './components/PlayURL.js';
import { TTS } from './components/TTS.js';
import { api } from './api.js';

const html = htm.bind(h);

function DeviceDetail({ deviceId, devices, onBack }) {
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
            <${Sources} deviceId=${deviceId} status=${device.status} />
            <${Zone} deviceId=${deviceId} devices=${devices} />
            <${Recents} deviceId=${deviceId} />
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
                setDevices(msg.data || {});
            } else if (msg.type === 'discovery_status') {
                console.log('[DEBUG_LOG] discovery_status:', msg.data);
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
                setDevices(prev => {
                    if (!prev[msg.deviceId]) return prev;
                    return {
                        ...prev,
                        [msg.deviceId]: { ...prev[msg.deviceId], status: msg.data },
                    };
                });
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

    async function removeDevice(id) {
        const name = devices[id]?.info?.name || id;
        if (!confirm(`Remove "${name}"?\n\nThis clears it from AfterTouch. A device still online may reappear after the next discovery scan.`)) {
            return;
        }
        // Optimistically drop it; the server's devices broadcast reconciles.
        setDevices(prev => {
            const next = { ...prev };
            delete next[id];
            return next;
        });
        try {
            const resp = await api.removeDevice(id);
            showToast(resp?.success ? `Removed "${name}"` : (resp?.error || 'Failed to remove device'));
        } catch (err) {
            showToast('Failed to remove device');
        }
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
                </div>
            </nav>

            <main class="main-content">
                ${page === 'devices' ? html`
                    <${DeviceList}
                        key="device-list"
                        devices=${devices}
                        isDiscovering=${isDiscovering}
                        onSelect=${(id) => navigate('device', id)}
                        onDiscover=${discover}
                        onRemove=${removeDevice}
                    />
                ` : page === 'device' ? html`
                    <${DeviceDetail}
                        key="device-detail"
                        deviceId=${selectedId}
                        devices=${devices}
                        onBack=${() => navigate('devices')}
                    />
                ` : page === 'tunein' ? html`
                    <${TuneInBrowser} key="tunein-browser" devices=${devices} />
                ` : page === 'radiobrowser' ? html`
                    <${RadioBrowser} key="radiobrowser-browser" devices=${devices} />
                ` : page === 'playurl' ? html`
                    <${PlayURL} key="play-url" devices=${devices} serverServiceUrl=${version?.service_url || ''} />
                ` : page === 'tts' ? html`
                    <${TTS} key="tts" devices=${devices} serverServiceUrl=${version?.service_url || ''} />
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

            ${toast ? html`<div class="toast" key="toast">${toast}</div>` : null}
        </div>
    `;
}

render(html`<${App} />`, document.getElementById('app'));
