import { h } from 'preact';
import htm from 'htm';

const html = htm.bind(h);

function DeviceCard({ id, device, onSelect }) {
    const { info, status } = device;
    const np = status?.nowPlaying;
    const isPlaying = np?.PlayStatus === 'PLAY_STATE';
    const isStandby = !np || np.Source === 'STANDBY';

    return html`
        <div class="device-card" onClick=${() => onSelect(id)}>
            <div class="device-header">
                <span class="device-name">${info?.Name || id}</span>
                <span class="device-indicator ${status?.isConnected ? 'online' : 'offline'}"></span>
            </div>
            <div class="device-type">${info?.Type || ''}</div>
            ${!isStandby && html`
                <div class="now-playing-mini">
                    <span class="play-status">${isPlaying ? '▶' : '⏸'}</span>
                    <span class="track-mini">${np.Track || np.StationName || np.Source}</span>
                    ${np.Artist && html`<span class="artist-mini"> — ${np.Artist}</span>`}
                </div>
            `}
            ${isStandby && html`<div class="standby-label">Standby</div>`}
        </div>
    `;
}

export function DeviceList({ devices, onSelect, onDiscover }) {
    const entries = Object.entries(devices);

    return html`
        <div class="page-header">
            <h2>Devices</h2>
            <button class="btn-secondary" onClick=${onDiscover}>Discover</button>
        </div>
        ${entries.length === 0
            ? html`
                <div class="empty-state">
                    <div class="empty-icon">◉</div>
                    <p>No devices found on your network.</p>
                    <button class="btn-primary" onClick=${onDiscover}>Start Discovery</button>
                </div>`
            : html`
                <div class="device-grid">
                    ${entries.map(([id, device]) => html`
                        <${DeviceCard} key=${id} id=${id} device=${device} onSelect=${onSelect} />
                    `)}
                </div>`
        }
    `;
}