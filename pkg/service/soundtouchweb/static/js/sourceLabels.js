// Display names for the speaker's source identifiers. Shared, because the
// preset tiles and the catalog picker have to name the same source the same
// way: a tile reading "Library" next to a picker row reading "STORED_MUSIC"
// would read as two different things.
const SOURCE_LABELS = {
    TUNEIN: 'TuneIn', SPOTIFY: 'Spotify', AMAZON: 'Amazon',
    PANDORA: 'Pandora', IHEARTRADIO: 'iHeart', DEEZER: 'Deezer',
    LOCAL_INTERNET_RADIO: 'Internet Radio',
    // A folder or track from a DLNA/NAS media server. Without this the tile
    // showed the raw source name (issue 700).
    STORED_MUSIC: 'Library',
    RADIO_BROWSER: 'Radio Browser',
};

export function sourceLabel(source) {
    return SOURCE_LABELS[source] || source;
}
