import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
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

    useEffect(() => {
        let cancelled = false;

        api.sourcesElsewhere(deviceId)
            .then(res => { if (!cancelled) setPayload(res?.data ?? null); })
            .catch(() => { if (!cancelled) setPayload(null); });

        return () => { cancelled = true; };
    }, [deviceId]);

    const sources = payload?.sources ?? [];

    // Nothing to say on a single-speaker setup, or where no service backs the
    // player. An empty section would only be noise.
    if (!payload?.available || sources.length === 0) return null;

    return html`
        <div class="sources-elsewhere">
            <h4 class="sources-elsewhere-title">On your other speakers</h4>
            <ul class="sources-elsewhere-list">
                ${sources.map(source => html`
                    <li key=${`${source.type}:${source.account || ''}`} class="sources-elsewhere-row">
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
                    </li>
                `)}
            </ul>
        </div>
    `;
}
