import { h } from 'preact';
import { useLayoutEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { SourceIcon } from '../sourceIcons.js';
import { sourceLabel } from '../sourceLabels.js';
import { CatalogPicker } from './CatalogPicker.js';
import { StoredPresets } from './StoredPresets.js';
import { sameContentLocation } from '../contentLocation.mjs';
import { currentPresetContent, verifyPresetEdit } from '../presetMove.mjs';
import { usePresetTarget } from '../usePresetTarget.js';

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
function PresetSlot({ preset, deviceId, active, canSave, command, commandBusy, onSelect, onEdit, editing, onStore, onStoreError, isCurrent }) {
    const [saveState, setSaveState] = useState(null);
    const resetTimer = useRef(null);
    useLayoutEffect(() => () => clearTimeout(resetTimer.current), []);

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
        resetTimer.current = setTimeout(() => setSaveState(null), 1500);
    }

    async function store() {
        if (!canSave || saveState === 'saving') return;
        clearTimeout(resetTimer.current);
        setSaveState('saving');
        try {
            await onStore(preset);
            if (isCurrent()) flash('saved');
        } catch (error) {
            if (isCurrent()) {
                flash('error');
                onStoreError(error.message);
            }
        }
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
                disabled=${saveState === 'saving' || (!savableEmpty && (isEmpty || commandBusy))}
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
                    disabled=${!canSave || saveState === 'saving'}
                    aria-busy=${saveState === 'saving' ? 'true' : null}
                    aria-label=${saveTitle}
                >
                    ${saveState === 'saved' ? '✓' : saveState === 'error' ? '✗' : mapped ? '★' : '☆'}
                </button>
            </span>
        </div>
    `;
}

export function Presets({ deviceId, physicalId, status, command, commandBusy = false, onSelect }) {
    // Which slot the catalog picker is open for, if any. One picker for the
    // whole section rather than a popover per tile: six tiles wrap to two or
    // three rows on a phone, and a list that has to be filtered needs the
    // width of the section, not the width of a tile.
    const [editingSlot, setEditingSlot] = useState(null);
    const [saveError, setSaveError] = useState(null);
    const presetTarget = usePresetTarget(deviceId, physicalId, status?.epoch);

    const presets = status?.presets?.Preset ?? [];
    const currentSource = status?.nowPlaying?.Source;
    const currentLocation = status?.nowPlaying?.ContentItem?.Location;

    const currentContent = currentPresetContent(status?.nowPlaying);
    const canSave = !!currentContent;

    // Build a map for quick lookup, then render slots 1-6
    const byId = Object.fromEntries(presets.map(p => [p.ID, p]));
    const slots = [1, 2, 3, 4, 5, 6].map(id => byId[id] ?? { ID: id, ContentItem: null });

    function isActive(preset) {
        const item = preset.ContentItem;
        return item && item.Source === currentSource &&
            sameContentLocation(currentSource, item.Location, currentLocation);
    }

    function store(preset) {
        setSaveError(null);
        return verifyPresetEdit({ api, ...presetTarget, slot: Number(preset.ID), content: currentContent, expected: preset.ContentItem });
    }

    return html`
        <div class="presets-section">
            <h3 class="section-title">Presets</h3>
            <${StoredPresets} key=${JSON.stringify([deviceId, physicalId, status?.epoch])}
                deviceId=${deviceId} physicalId=${physicalId} epoch=${status?.epoch} revision=${status?.revision} />
            <div class="preset-grid">
                ${slots.map(preset => html`
                    <${PresetSlot}
                        key=${JSON.stringify([deviceId, physicalId, status?.epoch, preset.ID])}
                        preset=${preset}
                        deviceId=${deviceId}
                        active=${isActive(preset)}
                        canSave=${canSave}
                        command=${command}
                        commandBusy=${commandBusy}
                        onSelect=${onSelect}
                        onStore=${store}
                        onStoreError=${setSaveError}
                        isCurrent=${presetTarget.isCurrent}
                        onEdit=${setEditingSlot}
                        editing=${editingSlot === preset.ID}
                    />
                `)}
            </div>
            ${saveError && html`<p class="settings-error" role="alert">${saveError}</p>`}
            ${editingSlot !== null && html`
                <${CatalogPicker}
                    key=${JSON.stringify([deviceId, physicalId, status?.epoch, editingSlot])}
                    deviceId=${deviceId}
                    physicalId=${physicalId}
                    epoch=${status?.epoch}
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
