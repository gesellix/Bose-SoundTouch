import { h } from 'preact';
import { useState } from 'preact/hooks';
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

// PresetSlot renders a single preset button plus, when something presetable
// is currently playing (canSave=true), an overlaid save button that stores
// the current content to that slot.  The save button lives *outside* the
// play button in the DOM (via the wrapper div) to avoid invalid nested
// <button> elements.
function PresetSlot({ preset, deviceId, active, canSave }) {
    const [saveState, setSaveState] = useState(null); // null | 'saved' | 'error'

    const item = preset?.ContentItem;
    const isEmpty = !item;
    const art = item?.ContainerArt;
    const name = item?.ItemName || `Preset ${preset?.ID ?? ''}`;

    function select() {
        if (!isEmpty) api.control(deviceId, 'preset', preset.ID);
    }

    function save(e) {
        e.stopPropagation();
        api.storePreset(deviceId, preset.ID)
            .then(res => {
                setSaveState(res.success ? 'saved' : 'error');
                setTimeout(() => setSaveState(null), 1500);
            })
            .catch(() => {
                setSaveState('error');
                setTimeout(() => setSaveState(null), 1500);
            });
    }

    return html`
        <div class="preset-slot-wrap">
            <button
                class="preset-slot ${isEmpty ? 'empty' : ''} ${active ? 'active' : ''}"
                data-source=${item?.Source ?? ''}
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
            ${canSave && html`
                <button
                    class="preset-save-btn ${saveState ?? ''}"
                    onClick=${save}
                    title="Save current to preset ${preset.ID}"
                    aria-label="Save current to preset ${preset.ID}"
                >
                    ${saveState === 'saved' ? '✓' : saveState === 'error' ? '✗' : '+'}
                </button>
            `}
        </div>
    `;
}

export function Presets({ deviceId, status }) {
    const presets = status?.presets?.Preset ?? [];
    const currentSource = status?.nowPlaying?.Source;
    const currentLocation = status?.nowPlaying?.ContentItem?.Location;

    // Show save buttons whenever something that isn't STANDBY is playing.
    // The server validates IsPresetable; if it fails the button shows ✗ briefly.
    const canSave = !!(currentSource && currentSource !== 'STANDBY');

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
                        canSave=${canSave}
                    />
                `)}
            </div>
        </div>
    `;
}