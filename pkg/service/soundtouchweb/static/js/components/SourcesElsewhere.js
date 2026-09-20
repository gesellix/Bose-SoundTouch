import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { SourceIcon } from '../sourceIcons.js';
import { sourceLabel } from '../sourceLabels.js';

const html = htm.bind(h);

// What each availability means for this speaker, in the owner's terms. The
// distinction is not cosmetic: a media server can be added for you because it
// is identified by an address and carries no credential, while a music service
// cannot, because the credential belongs to the speaker that acquired it.
const AVAILABILITY = {
    addable: 'can be added here',
    'link-required': 'needs linking on this speaker',
};

// SourcesElsewhere lists what this service's other speakers have and this one
// does not (issue 754): the media server one speaker discovered, the service
// linked on another.
//
// Read-only for now. It says what is possible before anything can act on it,
// because the difference between "can be added" and "needs linking here" is
// the thing an owner has to understand first.
export function SourcesElsewhere({ deviceId }) {
    const [payload, setPayload] = useState(null);
    const [busy, setBusy] = useState(null);
    const [error, setError] = useState(null);

    const load = useCallback(() => api.sourcesElsewhere(deviceId)
        .then(res => setPayload(res?.data ?? null))
        .catch(() => setPayload(null)), [deviceId]);

    useEffect(() => { load(); }, [load]);

    function add(source) {
        setBusy(keyOf(source));
        setError(null);

        api.addSourceElsewhere(deviceId, {
            type: source.type,
            account: source.account ?? '',
            name: source.display_name ?? '',
        })
            .then(res => {
                setBusy(null);

                if (res?.success === false) {
                    setError(res.error || 'The speaker refused that source.');

                    return;
                }

                // The list is derived from what the speaker has, so re-reading
                // it is how the added source disappears from "elsewhere".
                return load();
            })
            .catch(() => {
                setBusy(null);
                setError('The request did not complete; reload to see what this speaker has now.');
            });
    }

    const sources = payload?.sources ?? [];

    // Nothing to say on a single-speaker setup, or where no service backs the
    // player. An empty section would only be noise.
    if (!payload?.available || sources.length === 0) return null;

    return html`
        <div class="sources-elsewhere">
            <h4 class="sources-elsewhere-title">On your other speakers</h4>
            ${error && html`<p class="sources-elsewhere-error" role="alert">${error}</p>`}
            <ul class="sources-elsewhere-list">
                ${sources.map(source => html`
                    <li key=${keyOf(source)} class="sources-elsewhere-row">
                        <${SourceIcon} source=${source.type} className="sources-elsewhere-icon" />
                        <span class="sources-elsewhere-text">
                            <span class="sources-elsewhere-name">
                                ${source.display_name || sourceLabel(source.type)}
                            </span>
                            <span class="sources-elsewhere-meta">
                                ${sourceLabel(source.type)}
                                ${AVAILABILITY[source.availability]
                                    ? html` · ${AVAILABILITY[source.availability]}`
                                    : null}
                            </span>
                        </span>
                        ${source.availability === 'addable' && html`
                            <button
                                type="button"
                                class="sources-elsewhere-add"
                                disabled=${busy !== null}
                                title=${`Add ${source.display_name || sourceLabel(source.type)} to this speaker`}
                                onClick=${() => add(source)}
                            >${busy === keyOf(source) ? 'Adding…' : 'Add'}</button>
                        `}
                    </li>
                `)}
            </ul>
        </div>
    `;
}

function keyOf(source) {
    return `${source.type}:${source.account || ''}`;
}
