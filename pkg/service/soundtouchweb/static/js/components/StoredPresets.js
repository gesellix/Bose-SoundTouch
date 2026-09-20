import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { sourceLabel } from '../sourceLabels.js';

const html = htm.bind(h);

// Why a stored row cannot be recalled, in the owner's terms rather than the
// verdict's. The speaker is the authority here: it has six buttons, and a row
// naming anything else is a row it will never play.
const VERDICTS = {
    'out-of-range': 'no such button on this speaker',
    'no-slot': 'no button number at all',
    'empty-content': 'nothing to play',
};

// StoredPresets surfaces a disagreement between what AfterTouch stores for a
// speaker and what the speaker reports, and lets it be repaired (issue 697).
//
// It is silent unless something is actually wrong. A healthy install shows
// nothing at all, because a warning that is always there stops being read.
//
// The repair is the one preset write that goes to the service rather than
// through the speaker: these rows exist only in AfterTouch, and several of
// them name no button the speaker could be asked about.
export function StoredPresets({ deviceId, revision }) {
    const [payload, setPayload] = useState(null);
    const [open, setOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);

    const load = useCallback(() => api.storedPresets(deviceId)
        .then(res => setPayload(res?.data ?? null))
        .catch(() => setPayload(null)), [deviceId]);

    useEffect(() => { load(); }, [load, revision]);

    if (!payload?.disagrees) return null;

    const rows = payload.rows ?? [];
    const unrecallable = rows.filter(row => row.verdict !== 'ok');
    const slots = payload.slots ?? [];

    // Taking the speaker's side is a service-side write: what the speaker
    // reports for that button becomes what AfterTouch stores.
    function takeTheirs(slot) {
        setBusy(true);
        setError(null);

        api.adoptSpeakerPreset(deviceId, slot)
            .then(res => {
                setBusy(false);

                if (res?.success === false) {
                    setError(res.error || 'The speaker refused that slot.');

                    return;
                }

                setPayload(res?.data ?? null);
            })
            .catch(() => {
                setBusy(false);
                setError('The request did not complete; reload to see what is stored now.');
            });
    }

    // Keeping ours is a write to the SPEAKER: the stored list already wins at
    // the speaker's next fetch, so the only thing missing is the speaker
    // catching up now. It goes through the ordinary preset store, so it is
    // validated like any other (the speaker must have the source).
    function keepOurs(slot) {
        const content = slots.find(entry => entry.slot === slot)?.ours;

        if (!content?.present) return;

        setBusy(true);
        setError(null);

        api.storePresetContent(deviceId, slot, {
            source: content.source,
            sourceAccount: content.sourceAccount ?? '',
            location: content.location,
            type: content.type ?? '',
            itemName: content.itemName ?? '',
            containerArt: content.containerArt ?? '',
        })
            .then(res => {
                setBusy(false);

                if (res?.success === false) {
                    setError(res.error || 'The speaker refused that preset.');

                    return;
                }

                return load();
            })
            .catch(() => {
                setBusy(false);
                setError('The request did not complete; reload to see what the speaker has now.');
            });
    }

    function repair(drop) {
        setBusy(true);
        setError(null);

        api.repairStoredPresets(deviceId, drop, rows.length)
            .then(res => {
                setBusy(false);

                if (res?.success === false) {
                    // A refused repair is a stale view: reload and let the
                    // owner decide again against what is actually stored.
                    setError(res.error || 'The repair was refused.');
                    load();

                    return;
                }

                setPayload(res?.data ?? null);
            })
            .catch(() => {
                setBusy(false);
                setError('The request did not complete; reload to see what is stored now.');
                load();
            });
    }

    // Two different problems, so two different sentences. Rows the speaker can
    // never play are a fault in the stored list; a list that is merely longer
    // than the speaker's is not faulty row by row, it just cannot be imported
    // without losing something.
    const summary = unrecallable.length > 0
        ? `AfterTouch stores ${rows.length} presets for this speaker, ${unrecallable.length} of which it can never play.`
        : rows.length !== payload.speaker_count
            ? `AfterTouch stores ${rows.length} presets for this speaker; the speaker itself reports ${payload.speaker_count}.`
            : `${slots.length} preset${slots.length === 1 ? '' : 's'} hold${slots.length === 1 ? 's' : ''} different things in AfterTouch and on the speaker.`;

    return html`
        <section class="stored-presets" aria-label="Stored preset problems">
            <button
                type="button"
                class="stored-presets-summary ${open ? 'open' : ''}"
                onClick=${() => setOpen(o => !o)}
                aria-expanded=${open ? 'true' : 'false'}
            >
                <span class="stored-presets-mark" aria-hidden="true">!</span>
                <span class="stored-presets-text">
                    <span class="stored-presets-headline">${summary}</span>
                    <span class="stored-presets-hint">
                        ${open ? 'Hide what is stored' : 'Show what is stored'}
                    </span>
                </span>
            </button>

            ${open && html`
                <div class="stored-presets-body">
                    <p class="stored-presets-note">
                        A stored list that does not match the speaker's is why a "Sync Data" is
                        refused as destructive, and why the speaker can keep being handed the old
                        list.
                    </p>
                    <p class="stored-presets-note">
                        <strong>Remove</strong> deletes the row from what AfterTouch stores. It does
                        not press anything on the speaker, and it does not lose the station: that
                        stays in the list you pick from when filling a slot.
                    </p>

                    ${error && html`<p class="stored-presets-note error" role="alert">${error}</p>`}

                    ${slots.length > 0 && html`
                        <div class="stored-presets-slots">
                            <p class="stored-presets-note">
                                These buttons hold different things on each side. Pick one per
                                button; the other stays in the list you fill slots from.
                            </p>
                            ${slots.map(entry => html`
                                <div key=${entry.slot} class="stored-presets-slot">
                                    <span class="stored-presets-button">${entry.slot}</span>
                                    <span class="stored-presets-side">
                                        <span class="stored-presets-side-label">AfterTouch</span>
                                        <span class="stored-presets-side-name">
                                            ${entry.ours?.present ? (entry.ours.itemName || entry.ours.location) : 'nothing'}
                                        </span>
                                        <button
                                            type="button"
                                            class="stored-presets-choose"
                                            disabled=${busy || !entry.ours?.present}
                                            title=${entry.ours?.present
                                                ? `Put what AfterTouch stores into the speaker's preset ${entry.slot}`
                                                : 'AfterTouch stores nothing for this button'}
                                            onClick=${() => keepOurs(entry.slot)}
                                        >Keep ours</button>
                                    </span>
                                    <span class="stored-presets-side">
                                        <span class="stored-presets-side-label">Speaker</span>
                                        <span class="stored-presets-side-name">
                                            ${entry.theirs?.present ? (entry.theirs.itemName || entry.theirs.location) : 'nothing'}
                                        </span>
                                        <button
                                            type="button"
                                            class="stored-presets-choose"
                                            disabled=${busy || !entry.theirs?.present}
                                            title=${entry.theirs?.present
                                                ? `Store what the speaker has in preset ${entry.slot}`
                                                : 'The speaker holds nothing in this button'}
                                            onClick=${() => takeTheirs(entry.slot)}
                                        >Take the speaker's</button>
                                    </span>
                                </div>
                            `)}
                        </div>
                    `}

                    <ul class="stored-presets-list">
                        ${rows.map(row => html`
                            <li key=${row.index} class="stored-presets-row ${row.verdict === 'ok' ? '' : 'broken'}">
                                <span class="stored-presets-button">${row.button || '—'}</span>
                                <span class="stored-presets-detail">
                                    <span class="stored-presets-name">${row.name || row.location || 'Unnamed'}</span>
                                    <span class="stored-presets-meta">
                                        ${sourceLabel(row.source) || 'No source'}
                                        ${VERDICTS[row.verdict] ? html` · ${VERDICTS[row.verdict]}` : null}
                                    </span>
                                </span>
                                <button
                                    type="button"
                                    class="stored-presets-remove"
                                    disabled=${busy}
                                    title=${`Remove row ${row.index + 1} from what AfterTouch stores. The speaker's own buttons are not touched.`}
                                    aria-label=${`Remove ${row.name || row.location || 'this row'} from what AfterTouch stores`}
                                    onClick=${() => repair([row.index])}
                                >Remove</button>
                            </li>
                        `)}
                    </ul>

                    ${unrecallable.length > 0 && html`
                        <button
                            type="button"
                            class="stored-presets-fix"
                            disabled=${busy}
                            onClick=${() => repair(unrecallable.map(row => row.index))}
                        >
                            Remove ${unrecallable.length === 1 ? 'that row' : `those ${unrecallable.length} rows`} from AfterTouch
                        </button>
                    `}
                </div>
            `}
        </section>
    `;
}
