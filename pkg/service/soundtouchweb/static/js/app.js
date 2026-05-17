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
                <h2>${device.info?.Name || deviceId}</h2>
                <button class="btn-icon" onClick=${() => api.power(deviceId)} title="Power">⏻</button>
            </div>
            <${NowPlaying} nowPlaying=${device.status?.nowPlaying} />
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

    useEffect(() => {
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(`${protocol}//${location.host}/ws`);
        let reconnectTimer;

        ws.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            if (msg.type === 'devices') {
                setDevices(msg.data || {});
            } else if (msg.type === 'discovery_status') {
                if (msg.data?.status === 'completed') {
                    showToast(`Found ${msg.data.deviceCount} device(s)`);
                }
            } else if (msg.type === 'status_update' && msg.deviceId) {
                setDevices(prev => ({
                    ...prev,
                    [msg.deviceId]: { ...prev[msg.deviceId], status: msg.data },
                }));
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
        setToast(msg);
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

    return html`
        <div class="app">
            <nav class="navbar">
                <a class="brand" href="#" onClick=${(e) => { e.preventDefault(); navigate('devices'); }}>
                    SoundTouch
                </a>
                <div class="nav-links">
                    <a href="#" class="${page === 'devices' || page === 'device' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('devices'); }}>
                        Devices
                    </a>
                    <a href="#" class="${page === 'tunein' ? 'active' : ''}"
                        onClick=${(e) => { e.preventDefault(); navigate('tunein'); }}>
                        <img src="/static/img/tunein-mono.svg" alt="TuneIn" class="nav-tunein-icon" />
                    </a>
                    <button class="btn-icon" onClick=${discover} title="Discover">⟳</button>
                </div>
            </nav>

            <main class="main-content">
                ${page === 'devices' && html`
                    <${DeviceList}
                        devices=${devices}
                        onSelect=${(id) => navigate('device', id)}
                        onDiscover=${discover}
                    />
                `}
                ${page === 'device' && html`
                    <${DeviceDetail}
                        deviceId=${selectedId}
                        devices=${devices}
                        onBack=${() => navigate('devices')}
                    />
                `}
                ${page === 'tunein' && html`
                    <${TuneInBrowser} devices=${devices} />
                `}
            </main>

            ${toast && html`<div class="toast">${toast}</div>`}
        </div>
    `;
}

render(html`<${App} />`, document.getElementById('app'));