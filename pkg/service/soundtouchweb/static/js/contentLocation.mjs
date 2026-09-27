// Content locations the player compares to tell whether two items play the
// same thing (a recent and a preset, now-playing and a preset slot, a catalog
// entry and a slot).
//
// A LOCAL_INTERNET_RADIO station from Play URL has two spellings. Before
// issue 769 AfterTouch stored the absolute form,
// "<host>/core02/svc-bmx-adapter-orion/prod/orion/station?data=...", and since
// then the relative "/station?data=...", which the speaker resolves against
// the Orion baseUrl from its BMX registry. Both name the same station, so a
// list that mixes old and new entries must not treat them as different. This
// mirrors models.CanonicalContentLocation on the service side.

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

// canonicalLocation returns the location to compare: the relative form for an
// Orion station played from LOCAL_INTERNET_RADIO, the location unchanged for
// everything else.
export function canonicalLocation(source, location) {
    if ((source || '').toUpperCase() !== 'LOCAL_INTERNET_RADIO') return location || '';

    return relativeOrionLocation(location) ?? (location || '');
}

// sameContentLocation compares two locations of items from the same source.
export function sameContentLocation(source, left, right) {
    return canonicalLocation(source, left) === canonicalLocation(source, right);
}
