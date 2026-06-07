import { h } from 'preact';
import htm from 'htm';

const html = htm.bind(h);

function DeviceCard({ id, device, onSelect, onRemove }) {
    const { info, status } = device;
    const np = status?.nowPlaying;
    const isPlaying = np?.PlayStatus === 'PLAY_STATE';
    const isStandby = !np || np.Source === 'STANDBY';

    return html`
        <div class="device-card" onClick=${() => onSelect(id)}>
            <div class="device-header">
                <span class="device-name">${info?.name || id}</span>
                <span class="device-header-right">
                    <span class="device-indicator ${status?.isConnected ? 'online' : 'offline'}"></span>
                    <button class="device-remove" title="Remove this device"
                            aria-label="Remove this device"
                            onClick=${(e) => { e.stopPropagation(); onRemove(id); }}>✕</button>
                </span>
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

export function DeviceList({ devices, isDiscovering, onSelect, onDiscover, onRemove }) {
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
                        <${DeviceCard} key=${id} id=${id} device=${device} onSelect=${onSelect} onRemove=${onRemove} />
                    `)}
                </div>
                <p class="device-list-note" key="note">
                    Removing a device clears it here. One that is still online may
                    reappear after the next discovery scan.
                </p>`
        }
        </div>
    `;
}
