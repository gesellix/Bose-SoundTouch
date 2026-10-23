import { h } from 'preact';
import { useState } from 'preact/hooks';
import htm from 'htm';
import {
    connectivityLabel,
    connectivityState,
    nowPlayingFreshness,
    sortDeviceEntries,
} from '../devicePresentation.js';
import { zoneCardPresentation, zoneMembershipPresentation, zoneTopologyFingerprint } from '../zonePresentation.mjs';
import { ZoneMemberVolumeControl } from './ZoneMemberVolumeControl.js';
import { volumeProjectionKey } from '../zoneVolumeResult.mjs';
import { zoneMemberVolumes } from '../zoneVolumePreview.mjs';

const html = htm.bind(h);

const SORT_LS_KEY = 'aftertouch_device_sort';

function DeviceCard({ id, device, devices, onSelect }) {
    const { info, status } = device;
    const stereoPair = device.stereoPair;
    const zoneCard = device.zone ? zoneCardPresentation(device.zone) : null;
    const membership = zoneCard ? null : zoneMembershipPresentation(device.zoneMembership);
    const controlID = device.zone?.masterControlId || id;
    const np = status?.nowPlaying;
    const isPlaying = np?.PlayStatus === 'PLAY_STATE';
    const isStandby = !np || np.Source === 'STANDBY';
    const connectivity = connectivityState(device);
    const freshness = nowPlayingFreshness(device);
    const indicatorClass = zoneCard?.health || connectivity;
    const healthLabel = zoneCard?.healthLabel || `Connectivity: ${connectivityLabel(device)}`;

    const openButton = html`
        <button type="button" class=${zoneCard ? 'zone-card-open' : 'device-card'}
                onClick=${() => onSelect(controlID)}>
            <span class="device-header">
                <span class="device-name" title=${info?.name || id}>${info?.name || id}</span>
                <span class="device-indicator ${indicatorClass}" role="status"
                      title=${healthLabel} aria-label=${healthLabel}></span>
            </span>
            <span class="device-type">
                ${info?.type || ''}
                ${info?.ip_address ? html`<span class="device-ip">(${info.ip_address})</span>` : null}
                ${!zoneCard && stereoPair ? html`
                    <span class="stereo-pair-state ${stereoPair.degraded ? 'degraded' : ''}">
                        Stereo pair ${stereoPair.availableMemberCount}/${stereoPair.memberCount}
                    </span>
                ` : null}
            </span>
            ${zoneCard ? html`
                <span class="zone-card-summary">
                    <span class="zone-card-badge">${zoneCard.groupLabel}</span>
                    ${zoneCard.availabilityLabel ? html`
                        <span class="zone-card-availability" title=${zoneCard.availabilityTitle}>
                            ${zoneCard.availabilityLabel}
                        </span>
                    ` : null}
                </span>
            ` : null}
            ${membership ? html`
                <span class="zone-card-summary">
                    <span class="zone-member-badge ${membership.degraded ? 'degraded' : ''}"
                          title=${membership.title}>${membership.label}</span>
                </span>
            ` : null}
            ${!isStandby ? html`
                <span class="now-playing-mini ${freshness.live ? '' : 'unconfirmed'}"
                      title=${[
                          freshness.title,
                          [np.Track || np.StationName || np.Source, np.Artist].filter(Boolean).join(' - '),
                      ].filter(Boolean).join(': ')}>
                    ${freshness.live
                        ? html`<span class="play-status">${isPlaying ? '▶' : '⏸'}</span>`
                        : html`<span class="play-status">${freshness.label}:</span>`}
                    <span class="track-mini">${np.Track || np.StationName || np.Source}</span>
                    ${np.Artist ? html`<span class="artist-mini"> — ${np.Artist}</span>` : null}
                </span>
            ` : null}
            ${isStandby ? html`
                <span class="standby-label ${freshness.live ? '' : 'unconfirmed'}"
                      title=${freshness.title || null}>
                    ${freshness.live ? 'Standby' : `${freshness.label}: Standby`}
                </span>
            ` : null}
        </button>
    `;
    if (!zoneCard) return openButton;

    const zone = device.zone;
    const memberStatuses = (zone.members || []).map(member => [member.controlId,
        member.controlId === controlID ? status : devices[member.controlId]?.status]);
    const epoch = JSON.stringify(Object.fromEntries([[controlID, status?.epoch ?? null],
        ...memberStatuses.map(([id, memberStatus]) => [id, memberStatus?.epoch ?? null])]));
    const revision = JSON.stringify(Object.fromEntries([[controlID, status?.revision ?? null],
        ...memberStatuses.map(([id, memberStatus]) => [id, memberStatus?.revision ?? null])]));
    const topologyKey = JSON.stringify([epoch, zoneTopologyFingerprint(zone)]);
    const projectionKey = volumeProjectionKey(epoch, revision,
        zoneMemberVolumes(zone));
    return html`
        <section class="device-card zone-card ${zoneCard.health}"
                 aria-label=${info?.name || id}>
            ${openButton}
            <${ZoneMemberVolumeControl} key=${topologyKey}
                zoneMasterId=${controlID} topologyKey=${topologyKey}
                projectionKey=${projectionKey} group=${true}
                ariaLabel="Group volume" available=${zone.availableMemberCount !== 0}
                volume=${zone.volume} />
        </section>
    `;
}

export function DeviceList({ devices, isDiscovering, onSelect, onDiscover }) {
    const [sortMode, setSortMode] = useState(() => localStorage.getItem(SORT_LS_KEY) || 'ip');

    function changeSort(mode) {
        setSortMode(mode);
        localStorage.setItem(SORT_LS_KEY, mode);
    }

    const entries = sortDeviceEntries(Object.entries(devices), sortMode);

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
                        <${DeviceCard} key=${id} id=${id} device=${device} devices=${devices} onSelect=${onSelect} />
                    `)}
                </div>`
        }
        </div>
    `;
}
