import { h } from 'preact';
import { useState } from 'preact/hooks';
import htm from 'htm';

const html = htm.bind(h);

const SORT_LS_KEY = 'aftertouch_device_sort';

function sortEntries(entries, mode) {
    const copy = [...entries];
    if (mode === 'name') {
        // Sort by the speaker's display name, falling back to the map key (its IP)
        // when a device has no name yet.
        copy.sort(([idA, a], [idB, b]) =>
            (a?.info?.name || idA).localeCompare(b?.info?.name || idB, undefined, { sensitivity: 'base' }));
    } else {
        // Default: by IP (the map key), ordered numerically so .2 precedes .10.
        copy.sort(([idA], [idB]) =>
            idA.localeCompare(idB, undefined, { numeric: true, sensitivity: 'base' }));
    }
    return copy;
}

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
    const [sortMode, setSortMode] = useState(() => localStorage.getItem(SORT_LS_KEY) || 'ip');

    function changeSort(mode) {
        setSortMode(mode);
        localStorage.setItem(SORT_LS_KEY, mode);
    }

    const entries = sortEntries(Object.entries(devices), sortMode);

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
                <div class="device-sort" key="sort">
                    <span class="device-sort-label">Sort by</span>
                    <button class="sort-btn ${sortMode === 'name' ? 'active' : ''}"
                            onClick=${() => changeSort('name')}>Name</button>
                    <button class="sort-btn ${sortMode === 'ip' ? 'active' : ''}"
                            onClick=${() => changeSort('ip')}>IP</button>
                </div>
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
