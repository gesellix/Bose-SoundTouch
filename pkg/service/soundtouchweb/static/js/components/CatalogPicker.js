import { h } from 'preact';
import { useState, useEffect, useMemo } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { SourceIcon } from '../sourceIcons.js';
import { sourceLabel } from '../sourceLabels.js';
import { missingSourceFor } from '../sourceAvailability.mjs';

const html = htm.bind(h);

// CatalogPicker fills a preset slot from what AfterTouch has seen (issue 754).
//
// Recovering an emptied slot used to mean editing Presets.xml or files on the
// speaker over SSH. The catalog is the material that makes picking it again
// possible instead, so this list is the whole feature as far as an owner is
// concerned: stations and albums this household actually used, across
// speakers, rather than a location typed by hand.
//
// It deliberately does not hide entries that already sit in a slot. Copying
// one station into a second slot, or onto a second speaker, is a thing people
// do on purpose; the list only says where an entry already is.
export function CatalogPicker({ deviceId, slot, presets, sources = null, current = null, onClose, onAssigned }) {
    const [state, setState] = useState({ status: 'loading' });
    const [query, setQuery] = useState('');
    const [saving, setSaving] = useState(null);
    const [error, setError] = useState(null);
    const [confirmingClear, setConfirmingClear] = useState(false);
    const [pendingMove, setPendingMove] = useState(null);
    const [newName, setNewName] = useState(current?.ContentItem?.ItemName ?? '');

    const item = current?.ContentItem ?? null;
    const occupied = !!item;

    useEffect(() => {
        let cancelled = false;

        api.catalog()
            .then(res => {
                if (cancelled) return;

                const data = res?.data ?? {};

                setState({
                    status: data.available ? 'ready' : 'unavailable',
                    entries: data.entries ?? [],
                });
            })
            .catch(() => {
                if (!cancelled) setState({ status: 'error' });
            });

        return () => { cancelled = true; };
    }, []);

    // Which slot an entry already occupies, by the same content identity the
    // catalog is keyed on. Names are not part of it: the same name can belong
    // to two different stations on two sources.
    const slotByIdentity = useMemo(() => {
        const map = new Map();

        for (const preset of presets?.Preset ?? []) {
            const item = preset?.ContentItem;

            if (!item?.Location) continue;

            map.set(identityOf(item), preset.ID);
        }

        return map;
    }, [presets]);

    // What each slot currently holds, so a move can say what it would replace.
    const slotByNumber = useMemo(() => {
        const map = new Map();

        for (const preset of presets?.Preset ?? []) {
            if (preset?.ContentItem) map.set(Number(preset.ID), preset.ContentItem);
        }

        return map;
    }, [presets]);

    const entries = useMemo(() => {
        const needle = query.trim().toLowerCase();

        if (!needle) return state.entries ?? [];

        return (state.entries ?? []).filter(e =>
            (e.name || '').toLowerCase().includes(needle) ||
            sourceLabel(e.source).toLowerCase().includes(needle));
    }, [state.entries, query]);

    function pick(entry) {
        storeItem(slot, {
            source: entry.source,
            sourceAccount: entry.source_account || '',
            location: entry.location,
            type: entry.type || '',
            itemName: entry.name || '',
            containerArt: entry.container_art || '',
        }, entry.location)
            .then(() => {
                setSaving(null);
                onAssigned?.(entry);
            })
            .catch(failed('The request did not complete; the slot may or may not have changed.'));
    }

    // storeItem is the one write the editor makes: it names a ContentItem and
    // the slot it goes into. Filling from the catalog, renaming and moving are
    // all that write with a different item or a different slot, which is why
    // none of them needs its own endpoint.
    function storeItem(targetSlot, contentItem, key) {
        setSaving(key);
        setError(null);

        return api.storePresetContent(deviceId, targetSlot, contentItem)
            .then(res => {
                if (res?.success === false) {
                    throw new Error(res.error || 'The speaker refused this entry.');
                }

                return res;
            });
    }

    function failed(message) {
        return err => {
            setSaving(null);
            setError(err?.message || message);
        };
    }

    function itemPayload(overrides = {}) {
        return {
            source: item?.Source ?? '',
            sourceAccount: item?.SourceAccount ?? '',
            location: item?.Location ?? '',
            type: item?.Type ?? '',
            itemName: item?.ItemName ?? '',
            containerArt: item?.ContainerArt ?? '',
            ...overrides,
        };
    }

    function rename(e) {
        e.preventDefault();

        const name = newName.trim();

        if (!name || name === item?.ItemName) return;

        storeItem(slot, itemPayload({ itemName: name }), 'rename')
            .then(() => {
                setSaving(null);
                onAssigned?.(null);
            })
            .catch(failed('The request did not complete; the name may or may not have changed.'));
    }

    // A move is a store into the target followed by a clear of the source, in
    // that order: if the clear fails, the station is in two slots, which is
    // recoverable. The other order would risk losing it from both.
    function move(target) {
        if (slotByNumber.get(target) && pendingMove !== target) {
            setPendingMove(target);

            return;
        }

        storeItem(target, itemPayload(), `move:${target}`)
            .then(() => api.removePreset(deviceId, slot))
            .then(() => {
                setSaving(null);
                setPendingMove(null);
                onAssigned?.(null);
            })
            .catch(failed('The request did not complete; check both slots.'));
    }

    function clear() {
        setSaving('clear');
        setError(null);

        api.removePreset(deviceId, slot)
            .then(res => {
                setSaving(null);
                setConfirmingClear(false);

                if (res?.success === false) {
                    setError(res.error || 'The speaker refused to clear this slot.');

                    return;
                }

                onAssigned?.(null);
            })
            .catch(() => {
                setSaving(null);
                setError('The request did not complete; the slot may or may not have been cleared.');
            });
    }

    // A slot that holds something is being edited, not filled: renaming and
    // moving are right there, and "Fill" would name the least likely of the
    // three.
    const heading = occupied ? `Preset ${slot}` : `Fill preset ${slot}`;

    return html`
        <section class="catalog-picker" aria-label=${heading}>
            <header class="catalog-picker-head">
                <h4 class="catalog-picker-title">${heading}</h4>
                <button type="button" class="catalog-picker-close" onClick=${onClose} aria-label="Close">✕</button>
            </header>

            ${occupied && html`
                <div class="catalog-current">
                    <form class="catalog-rename" onSubmit=${rename}>
                        <label class="catalog-field-label" for=${`rename-${slot}`}>Name</label>
                        <input
                            id=${`rename-${slot}`}
                            class="catalog-rename-input"
                            type="text"
                            value=${newName}
                            disabled=${saving !== null}
                            onInput=${e => setNewName(e.target.value)}
                        />
                        <button
                            type="submit"
                            class="catalog-action-btn"
                            disabled=${saving !== null || !newName.trim() || newName.trim() === item.ItemName}
                        >Rename</button>
                    </form>
                    <div class="catalog-move">
                        <span class="catalog-field-label">Move to</span>
                        <span class="catalog-move-slots">
                            ${[1, 2, 3, 4, 5, 6].filter(n => n !== slot).map(n => html`
                                <button
                                    key=${n}
                                    type="button"
                                    class="catalog-move-slot ${slotByNumber.get(n) ? 'occupied' : ''} ${pendingMove === n ? 'pending' : ''}"
                                    disabled=${saving !== null}
                                    title=${slotByNumber.get(n)
                                        ? `Preset ${n} holds ${slotByNumber.get(n).ItemName || 'something'}`
                                        : `Move to the empty preset ${n}`}
                                    onClick=${() => move(n)}
                                >${n}</button>
                            `)}
                        </span>
                    </div>
                    ${pendingMove !== null && html`
                        <p class="catalog-picker-note" role="alert">
                            Preset ${pendingMove} holds ${slotByNumber.get(pendingMove)?.ItemName || 'something'}.
                            Press ${pendingMove} again to replace it; it stays on this list.
                        </p>
                    `}
                </div>
            `}

            ${state.status === 'loading' && html`<p class="catalog-picker-note">Loading…</p>`}

            ${state.status === 'error' && html`
                <p class="catalog-picker-note error">The catalog could not be loaded.</p>
            `}

            ${state.status === 'unavailable' && html`
                <p class="catalog-picker-note">
                    This player has no catalog behind it. It is kept by the AfterTouch
                    service, and can be switched off with the <code>catalog_size</code> setting.
                </p>
            `}

            ${state.status === 'ready' && (state.entries ?? []).length === 0 && html`
                <p class="catalog-picker-note">
                    Nothing seen yet. Presets and anything that plays are filed here as
                    they happen, and stay available once a slot is cleared.
                </p>
            `}

            ${state.status === 'ready' && (state.entries ?? []).length > 0 && html`
                <input
                    class="catalog-picker-filter"
                    type="search"
                    placeholder="Filter"
                    aria-label="Filter the catalog"
                    value=${query}
                    onInput=${e => setQuery(e.target.value)}
                />
                ${error && html`<p class="catalog-picker-note error" role="alert">${error}</p>`}
                <ul class="catalog-picker-list">
                    ${entries.map(entry => {
                        const occupies = slotByIdentity.get(identityOf({
                            Source: entry.source,
                            SourceAccount: entry.source_account,
                            Location: entry.location,
                        }));

                        // Advisory: the source list the player holds can be
                        // minutes old, so the entry stays clickable and the
                        // service decides against a fresh one.
                        const missing = missingSourceFor(sources, entry.source, entry.source_account);

                        return html`
                            <li key=${`${entry.source}:${entry.source_account || ''}:${entry.location}`}>
                                <button
                                    type="button"
                                    class="catalog-entry ${saving === entry.location ? 'saving' : ''} ${missing ? 'unavailable' : ''}"
                                    title=${missing ? `${entry.name || entry.location} — ${missing}` : null}
                                    disabled=${saving !== null}
                                    onClick=${() => pick(entry)}
                                >
                                    <span class="catalog-entry-thumb">
                                        ${entry.container_art
                                            ? html`<img src=${entry.container_art} alt="" />`
                                            : html`<${SourceIcon} source=${entry.source} className="catalog-entry-icon" />`}
                                    </span>
                                    <span class="catalog-entry-text">
                                        <span class="catalog-entry-name">${entry.name || entry.location}</span>
                                        <span class="catalog-entry-meta">
                                            ${sourceLabel(entry.source)}
                                            ${occupies !== undefined ? html` · in preset ${occupies}` : null}
                                            ${missing ? html` · <span class="catalog-entry-warn">${missing}</span>` : null}
                                        </span>
                                    </span>
                                </button>
                            </li>
                        `;
                    })}
                </ul>
                ${entries.length === 0 && html`<p class="catalog-picker-note">Nothing matches that filter.</p>`}
            `}

            ${occupied && html`
                <footer class="catalog-picker-foot">
                    ${confirmingClear
                        ? html`
                            <span class="catalog-picker-note">
                                Empty preset ${slot}? The station stays on this list.
                            </span>
                            <span class="catalog-picker-confirm">
                                <button type="button" class="catalog-clear-btn danger"
                                        disabled=${saving !== null}
                                        onClick=${clear}>Empty it</button>
                                <button type="button" class="catalog-clear-btn"
                                        disabled=${saving !== null}
                                        onClick=${() => setConfirmingClear(false)}>Keep it</button>
                            </span>
                        `
                        : html`
                            <button type="button" class="catalog-clear-btn"
                                    onClick=${() => setConfirmingClear(true)}>
                                Empty this slot
                            </button>
                        `}
                </footer>
            `}
        </section>
    `;
}

// identityOf mirrors the service's catalog key: source, the normalised account
// and location. The speaker echoes the source name back as sourceAccount in
// recents while leaving it empty in presets, so the raw values differ for one
// and the same station.
function identityOf(item) {
    const source = item?.Source || '';
    const account = item?.SourceAccount && item.SourceAccount !== source ? item.SourceAccount : '';

    return JSON.stringify([source, account, item?.Location || '']);
}
