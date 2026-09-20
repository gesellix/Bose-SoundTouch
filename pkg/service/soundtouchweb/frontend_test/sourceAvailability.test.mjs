import assert from 'node:assert/strict';
import test from 'node:test';

import { missingSourceFor } from '../static/js/sourceAvailability.mjs';

// Shapes taken from a SoundTouch 10: two media servers under STORED_MUSIC,
// one linked Spotify account, and TuneIn with no account at all.
const sources = {
    SourceItem: [
        { Source: 'TUNEIN', SourceAccount: '', Status: 'READY', DisplayName: 'TuneIn' },
        { Source: 'SPOTIFY', SourceAccount: 'listener', Status: 'READY' },
        { Source: 'STORED_MUSIC', SourceAccount: 'uuid:aaaa/0', Status: 'READY', DisplayName: 'MiniDLNA' },
    ],
};

test('content the speaker has is not flagged', () => {
    assert.equal(missingSourceFor(sources, 'TUNEIN', ''), '');
    assert.equal(missingSourceFor(sources, 'SPOTIFY', 'listener'), '');
    assert.equal(missingSourceFor(sources, 'STORED_MUSIC', 'uuid:aaaa/0'), '');
});

// Recents echo the source name back as the account; demanding a credential
// for that would flag every station the speaker plays happily.
test('the placeholder account is not treated as an account', () => {
    assert.equal(missingSourceFor(sources, 'TUNEIN', 'TUNEIN'), '');
});

test('a source the speaker does not have is named', () => {
    assert.match(missingSourceFor(sources, 'DEEZER', ''), /no Deezer source/);
});

test('an account the speaker does not have is flagged', () => {
    assert.match(missingSourceFor(sources, 'STORED_MUSIC', 'uuid:bbbb/0'), /Library accounts/);
    assert.match(missingSourceFor(sources, 'SPOTIFY', 'someone-else'), /Spotify accounts/);
});

// No list is not the same as an empty speaker: the service decides in that
// case, and flagging everything would make the marker meaningless.
test('nothing is flagged without a source list', () => {
    for (const empty of [undefined, null, {}, { SourceItem: [] }]) {
        assert.equal(missingSourceFor(empty, 'SPOTIFY', 'listener'), '');
    }
});
