import { h } from 'preact';
import { useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { SourceIcon } from '../sourceIcons.js';
import { sourceLabel } from '../sourceLabels.js';
import { CatalogPicker } from './CatalogPicker.js';
import { StoredPresets } from './StoredPresets.js';

const html = htm.bind(h);

// PresetSlot renders a single preset button plus an overlaid save button that
// stores the currently playing content into that slot.  The save button lives
// *outside* the play button in the DOM (via the wrapper div) to avoid invalid
// nested <button> elements.
//
// The save button is always rendered and merely disabled when nothing is
// playing, rather than hidden.  It used to be hover-revealed, which made it
// unreachable on touch devices and easy to miss everywhere else.  It shares
// the star glyph and the gold --fav colour with the now-playing preset
// picker (see NowPlaying.js) so both save paths read as the same gesture.
function PresetSlot({ preset, deviceId, active, canSave, command, commandBusy, onSelect, onEdit, editing }) {
    const [saveState, setSaveState] = useState(null); // null | 'saved' | 'error'

    const item = preset?.ContentItem;
    const isEmpty = !item;
    const art = item?.ContainerArt;
    const name = item?.ItemName || `Preset ${preset?.ID ?? ''}`;

    // An empty slot doubles as its own save target while something is
    // playing: there is nothing to overwrite, so the whole tile can invite
    // the save instead of hiding it behind the small corner button.
    const savableEmpty = isEmpty && canSave;

    // The star is always gold; only its fill carries meaning.  Solid ★ means
    // "what's playing right now is this preset" - the same thing the star on
    // the now-playing card means.  Outline ☆ is the neutral "save into this
    // slot" state.
    const mapped = !!active;

    function flash(state) {
        setSaveState(state);
        setTimeout(() => setSaveState(null), 1500);
    }

    function store() {
        api.storePreset(deviceId, preset.ID)
            .then(res => flash(res.success ? 'saved' : 'error'))
            .catch(() => flash('error'));
    }

    function select() {
        if (savableEmpty) store();
        else if (!isEmpty) (onSelect || (() => api.control(deviceId, 'preset', preset.ID)))(preset);
    }

    function save(e) {
        e.stopPropagation();
        store();
    }

    function edit(e) {
        e.stopPropagation();
        onEdit?.(editing ? null : preset.ID);
    }

    const saveTitle = !canSave
        ? 'Play something first, then save it to a preset'
        : mapped
            ? `Save again to preset ${preset.ID} (already saved there)`
            : `Save what's playing to preset ${preset.ID}`;

    // The star saves what is playing; the pencil fills the slot from the
    // catalog, which is how a slot that was emptied gets its station back
    // without anyone editing XML (issue 754).
    const editTitle = `Fill preset ${preset.ID} from the catalog`;

    const slotTitle = isEmpty
        ? (savableEmpty ? "Save what's playing here" : 'Empty')
        : name;

    return html`
        <div class="preset-slot-wrap ${active ? 'active' : ''}">
            <button
                class="preset-slot ${isEmpty ? 'empty' : ''} ${active ? 'active' : ''} ${savableEmpty ? 'savable' : ''}"
                data-source=${item?.Source ?? ''}
                onClick=${select}
                disabled=${!savableEmpty && (isEmpty || commandBusy)}
                aria-busy=${command?.action === 'preset' && commandBusy &&
                    command?.expected?.targetId === String(preset.ID) ? 'true' : null}
                title=${slotTitle}
            >
                <span class="preset-thumb">
                    ${art
                        ? html`<img class="preset-art" src=${art} alt="" />`
                        : savableEmpty
                            ? html`<span class="preset-thumb-glyph">☆</span>`
                            : isEmpty
                                ? null
                                : html`<${SourceIcon} source=${item.Source} className="preset-source-icon" />`
                    }
                </span>
                <span class="preset-text">
                    ${!isEmpty && html`<span class="preset-source-label">${sourceLabel(item.Source)}</span>`}
                    <span class="preset-name">
                        ${isEmpty ? (savableEmpty ? 'Save current here' : 'Empty') : name}
                    </span>
                </span>
                <span class="preset-num">${preset?.ID ?? ''}</span>
            </button>
            <span class="preset-edit-wrap" title=${editTitle}>
                <button
                    class="preset-edit-btn ${editing ? 'open' : ''}"
                    onClick=${edit}
                    aria-label=${editTitle}
                    aria-expanded=${editing ? 'true' : 'false'}
                >✎</button>
            </span>
            <span class="preset-save-wrap" title=${saveTitle}>
                <button
                    class="preset-save-btn ${mapped ? 'mapped' : ''} ${saveState ?? ''}"
                    onClick=${save}
                    disabled=${!canSave}
                    aria-label=${saveTitle}
                >
                    ${saveState === 'saved' ? '✓' : saveState === 'error' ? '✗' : mapped ? '★' : '☆'}
                </button>
            </span>
        </div>
    `;
}

export function Presets({ deviceId, status, command, commandBusy = false, onSelect }) {
    // Which slot the catalog picker is open for, if any. One picker for the
    // whole section rather than a popover per tile: six tiles wrap to two or
    // three rows on a phone, and a list that has to be filtered needs the
    // width of the section, not the width of a tile.
    const [editingSlot, setEditingSlot] = useState(null);

    const presets = status?.presets?.Preset ?? [];
    const currentSource = status?.nowPlaying?.Source;
    const currentLocation = status?.nowPlaying?.ContentItem?.Location;

    // Enable the save buttons whenever something that isn't STANDBY is
    // playing.  The server validates IsPresetable; if it fails the button
    // shows ✗ briefly.
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
            <${StoredPresets} deviceId=${deviceId} revision=${status?.revision} />
            <div class="preset-grid">
                ${slots.map(preset => html`
                    <${PresetSlot}
                        key=${preset.ID}
                        preset=${preset}
                        deviceId=${deviceId}
                        active=${isActive(preset)}
                        canSave=${canSave}
                        command=${command}
                        commandBusy=${commandBusy}
                        onSelect=${onSelect}
                        onEdit=${setEditingSlot}
                        editing=${editingSlot === preset.ID}
                    />
                `)}
            </div>
            ${editingSlot !== null && html`
                <${CatalogPicker}
                    key=${`catalog:${editingSlot}`}
                    deviceId=${deviceId}
                    slot=${editingSlot}
                    presets=${status?.presets}
                    sources=${status?.sources}
                    current=${byId[editingSlot] ?? null}
                    onClose=${() => setEditingSlot(null)}
                    onAssigned=${() => setEditingSlot(null)}
                />
            `}
        </div>
    `;
}
