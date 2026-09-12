// The speaker's /recents omits container art. Measured on a SoundTouch 10:
// all six presets carried a containerArt URL and not one of ten recents did,
// which matches what the captures under _/ have shown for other reporters
// (Spotify recents never carry art; TuneIn sometimes do). It is the speaker's
// doing: /presets, /recents and /now_playing all read artwork back from what
// the speaker was handed at /select time, and nothing hands it to recents.
//
// The player can still show something, because the same content is often a
// preset, and presets do carry art. This joins the two by content identity.
// It is best-effort by construction: an entry with no match keeps the source
// icon it has today, and nothing here can fail in a way that costs the list.

// identityOf keys a ContentItem by what actually identifies the content.
//
// The account needs normalising first: the speaker echoes the source name back
// as sourceAccount in recents ("TUNEIN", "RADIO_BROWSER") while writing an
// empty one in presets, so comparing the raw values misses every station. That
// placeholder pattern is the same one Sources.js normalises for source
// selection. On the measured speaker it is the difference between matching 4
// of 10 recents and matching 6.
//
// Item names are deliberately not part of the identity, and are not a
// fallback: the same speaker had "WDR 2 Rheinland" as both a RADIO_BROWSER
// preset and a TUNEIN recent, pointing at different stations. Borrowing art
// across those would show the wrong logo.
export function identityOf(item) {
    const source = item?.Source || '';
    const account = item?.SourceAccount && item.SourceAccount !== source
        ? item.SourceAccount
        : '';

    return JSON.stringify([source, account, item?.Location || '']);
}

// presetArtIndex maps content identity to artwork, for every preset that has
// both. Presets without art, or without a location to identify them by, are
// skipped rather than indexed as empty.
export function presetArtIndex(presets) {
    const index = new Map();

    for (const preset of presets?.Preset ?? []) {
        const item = preset?.ContentItem;

        if (!item?.ContainerArt || !item?.Location) continue;

        const key = identityOf(item);

        if (!index.has(key)) index.set(key, item.ContainerArt);
    }

    return index;
}

// artworkFor returns the artwork to show for a recents entry: its own if the
// speaker sent one, otherwise a preset's for the same content, otherwise an
// empty string so the caller falls back to its source icon.
export function artworkFor(item, index) {
    if (item?.ContainerArt) return item.ContainerArt;

    if (!item?.Location || !index?.size) return '';

    return index.get(identityOf(item)) || '';
}
