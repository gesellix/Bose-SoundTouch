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

    const summary = unrecallable.length > 0
        ? `AfterTouch stores ${rows.length} presets for this speaker, ${unrecallable.length} of which it can never play.`
        : `AfterTouch stores ${rows.length} presets for this speaker, which has ${payload.speaker_count} in its ${rows.length > 6 ? '6 buttons' : 'buttons'}.`;

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
                        Extra rows are why a "Sync Data" is refused as destructive, and why the
                        speaker keeps being handed the old list. Removing one here does not lose
                        the station: it stays in the preset list you pick from.
                    </p>

                    ${error && html`<p class="stored-presets-note error" role="alert">${error}</p>`}

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
                                    title=${`Remove the stored row at position ${row.index + 1}`}
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
                            Remove the ${unrecallable.length} row${unrecallable.length === 1 ? '' : 's'} this speaker cannot play
                        </button>
                    `}
                </div>
            `}
        </section>
    `;
}
