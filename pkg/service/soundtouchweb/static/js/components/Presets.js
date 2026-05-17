import { h } from 'preact';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

const SOURCE_LABELS = {
    TUNEIN: 'TuneIn', SPOTIFY: 'Spotify', AMAZON: 'Amazon',
    PANDORA: 'Pandora', IHEARTRADIO: 'iHeart', DEEZER: 'Deezer',
    LOCAL_INTERNET_RADIO: 'Internet Radio',
};

function sourceLabel(source) {
    return SOURCE_LABELS[source] || source;
}

function PresetSlot({ preset, deviceId, active }) {
    const item = preset?.ContentItem;
    const isEmpty = !item;
    const art = item?.ContainerArt;
    const name = item?.ItemName || `Preset ${preset?.ID ?? ''}`;

    function select() {
        if (!isEmpty) api.control(deviceId, 'preset', preset.ID);
    }

    return html`
        <button
            class="preset-slot ${isEmpty ? 'empty' : ''} ${active ? 'active' : ''}"
            onClick=${select}
            disabled=${isEmpty}
            title=${isEmpty ? 'Empty' : name}
        >
            ${art
                ? html`<img class="preset-art" src=${art} alt="" />`
                : html`<span class="preset-source-label">${isEmpty ? '—' : sourceLabel(item.Source)}</span>`
            }
            <span class="preset-name">${isEmpty ? 'Empty' : name}</span>
            <span class="preset-num">${preset?.ID ?? ''}</span>
        </button>
    `;
}

export function Presets({ deviceId, status }) {
    const presets = status?.presets?.Preset ?? [];
    const currentSource = status?.nowPlaying?.Source;
    const currentLocation = status?.nowPlaying?.ContentItem?.Location;

    // Build a map for quick lookup, then render slots 1-6
    const byId = Object.fromEntries(presets.map(p => [p.ID, p]));
    const slots = [1, 2, 3, 4, 5, 6].map(id => byId[id] ?? { ID: id, ContentItem: null });

    function isActive(preset) {
        const item = preset.ContentItem;
        return item && item.Source === currentSource && item.Location === currentLocation;
    }

    return html`
        <div class="presets-section">
            <h3 class="section-title">Presets</h3>
            <div class="preset-grid">
                ${slots.map(preset => html`
                    <${PresetSlot}
                        key=${preset.ID}
                        preset=${preset}
                        deviceId=${deviceId}
                        active=${isActive(preset)}
                    />
                `)}
            </div>
        </div>
    `;
}