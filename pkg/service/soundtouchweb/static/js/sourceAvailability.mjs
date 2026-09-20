import { sourceLabel } from './sourceLabels.js';

// A preset can name a source the target speaker does not have: a media server
// only one speaker has discovered, a music service linked on another one.
// Stored anyway, the slot looks fine and fails at play time (issue 754).
//
// This is the player's half of that check, against the source list it already
// holds, so a catalog entry can say so before anyone clicks it. It is
// advisory: the cached list can be minutes old, so the service re-checks
// against the speaker and has the final word. That is why an entry marked
// here is still clickable.
//
// Mirrors describeMissingSource in preset_source_check.go.
export function missingSourceFor(sources, source, account) {
    const items = sources?.SourceItem ?? [];

    if (!source || items.length === 0) return '';

    // The speaker echoes the source name back as sourceAccount where there is
    // no real account, and presets for the same station leave it empty.
    const wanted = (account ?? '').trim().toLowerCase() === source.trim().toLowerCase()
        ? ''
        : (account ?? '').trim();

    const matching = items.filter(item =>
        (item?.Source ?? '').trim().toLowerCase() === source.trim().toLowerCase());

    if (matching.length === 0) return `this speaker has no ${sourceLabel(source)} source`;

    if (!wanted) return '';

    const has = matching.some(item =>
        (item?.SourceAccount ?? '').trim().toLowerCase() === wanted.toLowerCase());

    return has ? '' : `not on this speaker's ${sourceLabel(source)} accounts`;
}
