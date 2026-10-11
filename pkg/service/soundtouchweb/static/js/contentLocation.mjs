// Content locations the player compares to tell whether two items play the
// same thing (a recent and a preset, now-playing and a preset slot, a catalog
// entry and a slot).
//
// A LOCAL_INTERNET_RADIO station from Play URL has two spellings. Before
// issue 769 AfterTouch stored the absolute form,
// "<host>/core02/svc-bmx-adapter-orion/prod/orion/station?data=...", and since
// then the relative "/station?data=...", which the speaker resolves against
// the Orion baseUrl from its BMX registry. Both name the same station, so a
// list that mixes old and new entries must not treat them as different. The
// older playback adapter ("<host>/custom/v1/playback/<base64 stream>") is a
// third spelling of the same thing. This mirrors models.CanonicalContentLocation
// on the service side.

const ORION_BASE_PATH = '/core02/svc-bmx-adapter-orion/prod/orion';
const ORION_STATION_PATH = '/station';

function relativeOrionLocation(location) {
    const trimmed = (location || '').trim();
    if (!trimmed) return null;

    if (trimmed === ORION_STATION_PATH || trimmed.startsWith(ORION_STATION_PATH + '?')) {
        return trimmed;
    }

    const match = /^[a-z][a-z0-9+.-]*:\/\/[^/?#]+(\/[^?#]*)(\?[^#]*)?$/i.exec(trimmed);
    if (!match || match[1] !== ORION_BASE_PATH + ORION_STATION_PATH) return null;

    return ORION_STATION_PATH + (match[2] && match[2] !== '?' ? match[2] : '');
}

const PLAYBACK_ADAPTER_PATH = '/custom/v1/playback/';
const STREAM_KEY_PREFIX = 'stream:';

// decodeBase64 accepts standard and URL-safe base64, padded or not, and
// returns the UTF-8 text, or null.
function decodeBase64(value) {
    const cleaned = (value || '').trim().replace(/-/g, '+').replace(/_/g, '/');
    if (!cleaned) return null;
    try {
        const padded = cleaned + '='.repeat((4 - (cleaned.length % 4)) % 4);
        const binary = atob(padded);
        const bytes = Uint8Array.from(binary, c => c.charCodeAt(0));
        return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
    } catch {
        return null;
    }
}

function looksLikeStreamURL(value) {
    try {
        const u = new URL(value);
        return (u.protocol === 'http:' || u.protocol === 'https:') && u.host !== '';
    } catch {
        return false;
    }
}

function orionStreamURL(relative) {
    const query = relative.includes('?') ? relative.slice(relative.indexOf('?') + 1) : '';
    const json = decodeBase64(new URLSearchParams(query).get('data'));
    if (!json) return null;
    try {
        const stream = (JSON.parse(json)?.streamUrl || '').trim();
        return stream || null;
    } catch {
        return null;
    }
}

function playbackAdapterStreamURL(location) {
    let path = location;
    try {
        path = new URL(location, 'http://placeholder.invalid').pathname;
    } catch {
        // keep the raw string
    }
    const at = path.indexOf(PLAYBACK_ADAPTER_PATH);
    if (at < 0) return null;
    let encoded = path.slice(at + PLAYBACK_ADAPTER_PATH.length);
    if (!encoded || encoded.includes('/')) return null;
    try {
        encoded = decodeURIComponent(encoded);
    } catch {
        // keep it escaped
    }
    // The adapter takes a base64-encoded URL and, failing that, a plain one.
    const decoded = decodeBase64(encoded);
    const stream = decoded && looksLikeStreamURL(decoded) ? decoded : encoded;
    return looksLikeStreamURL(stream) ? stream : null;
}

// stationStreamURL returns the stream URL a LOCAL_INTERNET_RADIO station
// location wraps (Orion relative or absolute, or the playback adapter), or
// null. Mirrors models.StationStreamURL.
export function stationStreamURL(location) {
    const trimmed = (location || '').trim();
    const relative = relativeOrionLocation(trimmed);
    if (relative !== null) return orionStreamURL(relative);
    return playbackAdapterStreamURL(trimmed);
}

// canonicalLocation returns the key to compare, never a location to store.
// A LOCAL_INTERNET_RADIO station is keyed by the stream it wraps, so its
// relative and absolute Orion spellings and the older playback adapter count
// as one station (issue 784). An Orion location without a decodable stream
// falls back to its relative form; everything else stays as it is.
export function canonicalLocation(source, location) {
    if ((source || '').toUpperCase() !== 'LOCAL_INTERNET_RADIO') return location || '';

    const stream = stationStreamURL(location);
    if (stream) return STREAM_KEY_PREFIX + stream;

    return relativeOrionLocation(location) ?? (location || '');
}

// sameContentLocation compares two locations of items from the same source.
export function sameContentLocation(source, left, right) {
    return canonicalLocation(source, left) === canonicalLocation(source, right);
}
