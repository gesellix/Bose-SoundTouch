import { h } from 'preact';
import htm from 'htm';

const html = htm.bind(h);

// One place deciding how a source is depicted, shared by the Presets, Recents
// and Sources views (each used to keep its own divergent emoji map).
//
// Two kinds of icon, deliberately:
//
//   * Monochrome SVGs for sources we can draw ourselves - the radio providers,
//     the physical inputs, a speaker for anything unknown. These are the same
//     assets the top navigation uses: black on transparent, recoloured for the
//     current theme through --content-icon-filter (and --content-icon-filter-alt
//     on inverted backgrounds like an active preset tile).
//
//   * Emoji for the streaming brands. Their logos are trademarks we would
//     rather not ship, and five identical generic notes would tell the reader
//     less than five distinguishable emoji do.
const SOURCE_ICON_FILES = {
    TUNEIN: 'tunein-mono.svg',
    RADIO_BROWSER: 'radiobrowser-mono.svg',
    LOCAL_INTERNET_RADIO: 'link-mono.svg',
    PRODUCT: 'speaker-mono.svg',
    AUX: 'aux-mono.svg',
    OPTICAL: 'optical-mono.svg',
    HDMI: 'hdmi-mono.svg',
    BLUETOOTH: 'bluetooth-mono.svg',
    AIRPLAY: 'airplay-mono.svg',
    STORED_MUSIC: 'disc-mono.svg',
    LOCAL_MUSIC: 'disc-mono.svg',
};

const SOURCE_EMOJI = {
    SPOTIFY: '🎵', AMAZON: '🛒', PANDORA: '🎶',
    DEEZER: '🎼', IHEARTRADIO: '❤️', IHEART: '❤️',
};

const FALLBACK_ICON_FILE = 'speaker-mono.svg';

// URL of the mono SVG for this source, or null when it is depicted by emoji.
export function sourceIconURL(source) {
    if (SOURCE_EMOJI[source]) return null;

    return `/app/static/img/${SOURCE_ICON_FILES[source] ?? FALLBACK_ICON_FILE}`;
}

// Emoji for this source, or null when it has a mono SVG.
export function sourceEmoji(source) {
    return SOURCE_EMOJI[source] ?? null;
}

// SourceIcon renders whichever of the two a source uses, so callers don't
// repeat the fallback dance.  The caller's class carries the sizing; the
// theme filter is scoped to img in the stylesheet, since it would wash an
// emoji out.
export function SourceIcon({ source, className }) {
    const url = sourceIconURL(source);

    return url
        ? html`<img class=${className} src=${url} alt="" />`
        : html`<span class=${className}>${sourceEmoji(source)}</span>`;
}
