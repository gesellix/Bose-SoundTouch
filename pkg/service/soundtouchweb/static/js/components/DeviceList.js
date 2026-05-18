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
                <span class="device-name">${info?.name || id}</span>
                <span class="device-indicator ${status?.isConnected ? 'online' : 'offline'}"></span>
            </div>
            <div class="device-type">
                ${info?.type || ''}
                ${info?.ip_address ? html`<span class="device-ip">(${info.ip_address})</span>` : null}
            </div>
            ${!isStandby ? html`
                <div class="now-playing-mini">
                    <span class="play-status">${isPlaying ? '▶' : '⏸'}</span>
                    <span class="track-mini">${np.Track || np.StationName || np.Source}</span>
                    ${np.Artist ? html`<span class="artist-mini"> — ${np.Artist}</span>` : null}
                </div>
            ` : null}
            ${isStandby ? html`<div class="standby-label">Standby</div>` : null}
        </div>
    `;
}

export function DeviceList({ devices, isDiscovering, onSelect, onDiscover }) {
    const entries = Object.entries(devices);

    return html`
        <div class="device-list-container">
        ${entries.length === 0
            ? html`
                <div class="empty-state" key="empty">
                    <div class="empty-icon ${isDiscovering ? 'radiating' : ''}">◉</div>
                    <p>${isDiscovering ? 'Searching for devices...' : 'No devices found on your network.'}</p>
                    <button class="btn-primary" onClick=${onDiscover} disabled=${isDiscovering}>
                        ${isDiscovering ? 'Discovering...' : 'Start Discovery'}
                    </button>
                </div>`
            : html`
                <div class="device-grid" key="grid">
                    ${entries.map(([id, device]) => html`
                        <${DeviceCard} key=${id} id=${id} device=${device} onSelect=${onSelect} />
                    `)}
                </div>`
        }
        </div>
    `;
}
