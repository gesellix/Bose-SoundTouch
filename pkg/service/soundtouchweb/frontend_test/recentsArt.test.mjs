import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

import { artworkFor, identityOf, presetArtIndex } from '../static/js/recentsArt.mjs';

// Shapes taken from a SoundTouch 10: presets carry containerArt, recents do
// not, and recents echo the source name back as the account.
const presets = {
    Preset: [
        {
            ID: 1,
            ContentItem: {
                Source: 'TUNEIN',
                SourceAccount: '',
                Location: '/v1/playback/station/s6634',
                ItemName: 'A Station',
                ContainerArt: 'http://example.invalid/station.png',
            },
        },
        {
            ID: 2,
            ContentItem: {
                Source: 'SPOTIFY',
                SourceAccount: 'listener',
                Location: '/playback/container/abc',
                ItemName: 'An Album',
                ContainerArt: 'http://example.invalid/album.jpg',
            },
        },
        {
            ID: 3,
            ContentItem: {
                Source: 'RADIO_BROWSER',
                SourceAccount: '',
                Location: '/stations/byuuid/1234',
                ItemName: 'Another Station',
                ContainerArt: '',
            },
        },
    ],
};

test('a recents entry borrows the artwork of the preset for the same content', () => {
    const index = presetArtIndex(presets);
    const recent = {
        Source: 'SPOTIFY',
        SourceAccount: 'listener',
        Location: '/playback/container/abc',
        ItemName: 'An Album',
    };

    assert.equal(artworkFor(recent, index), 'http://example.invalid/album.jpg');
});

// The speaker writes sourceAccount="TUNEIN" in recents and "" in presets for
// the same station. Comparing raw accounts misses every station this way.
test('the echoed source-name account matches an empty one', () => {
    const index = presetArtIndex(presets);
    const recent = {
        Source: 'TUNEIN',
        SourceAccount: 'TUNEIN',
        Location: '/v1/playback/station/s6634',
        ItemName: 'A Station',
    };

    assert.equal(artworkFor(recent, index), 'http://example.invalid/station.png');
});

test('a different location borrows nothing, even with the same name', () => {
    const index = presetArtIndex(presets);
    const recent = {
        Source: 'TUNEIN',
        SourceAccount: 'TUNEIN',
        Location: '/v1/playback/station/s213886',
        ItemName: 'A Station',
    };

    assert.equal(artworkFor(recent, index), '');
});

test('a different source borrows nothing', () => {
    const index = presetArtIndex(presets);
    const recent = {
        Source: 'RADIO_BROWSER',
        SourceAccount: 'RADIO_BROWSER',
        Location: '/v1/playback/station/s6634',
        ItemName: 'A Station',
    };

    assert.equal(artworkFor(recent, index), '');
});

test("an entry's own artwork always wins", () => {
    const index = presetArtIndex(presets);
    const recent = {
        Source: 'SPOTIFY',
        SourceAccount: 'listener',
        Location: '/playback/container/abc',
        ContainerArt: 'http://example.invalid/own.jpg',
    };

    assert.equal(artworkFor(recent, index), 'http://example.invalid/own.jpg');
});

test('presets without artwork are not indexed as empty', () => {
    const index = presetArtIndex(presets);

    assert.equal(index.size, 2);
    assert.equal(artworkFor({
        Source: 'RADIO_BROWSER',
        SourceAccount: '',
        Location: '/stations/byuuid/1234',
    }, index), '');
});

// Nothing here may throw: the list still has to render when a device has no
// presets, no recents content, or a half-filled entry.
test('missing and malformed input yields no artwork rather than an error', () => {
    assert.equal(presetArtIndex(undefined).size, 0);
    assert.equal(presetArtIndex({}).size, 0);
    assert.equal(presetArtIndex({ Preset: [null, {}, { ContentItem: {} }] }).size, 0);

    const index = presetArtIndex(presets);

    assert.equal(artworkFor(undefined, index), '');
    assert.equal(artworkFor({}, index), '');
    assert.equal(artworkFor({ Source: 'SPOTIFY' }, index), '');
    assert.equal(artworkFor({ Source: 'SPOTIFY', Location: '/x' }, undefined), '');
});

test('identity ignores nothing that distinguishes content', () => {
    const base = { Source: 'SPOTIFY', SourceAccount: 'listener', Location: '/a' };

    assert.notEqual(identityOf(base), identityOf({ ...base, Location: '/b' }));
    assert.notEqual(identityOf(base), identityOf({ ...base, Source: 'AMAZON' }));
    assert.notEqual(identityOf(base), identityOf({ ...base, SourceAccount: 'other' }));
});

// The panel must actually use the join, not just ship the helper.
test('the Recents panel resolves artwork through the preset index', async () => {
    const source = await readFile(new URL('../static/js/components/Recents.js', import.meta.url), 'utf8');

    assert.match(source, /presetArtIndex/);
    assert.match(source, /artworkFor/);
});
