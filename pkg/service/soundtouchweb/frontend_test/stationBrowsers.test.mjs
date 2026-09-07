import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const dir = new URL('../static/js/components/', import.meta.url);
const tuneIn = await readFile(new URL('TuneInBrowser.js', dir), 'utf8');
const radioBrowser = await readFile(new URL('RadioBrowser.js', dir), 'utf8');

// The speaker keeps the ContainerArt it is handed at select time, and presets,
// recents and now-playing all read the artwork back from there. Both browsers
// display the station logo in their result list, so there is no excuse for
// dropping it on the way to the speaker.
test('the TuneIn browser sends the station logo with the play request', () => {
    assert.match(tuneIn, /containerArt: pendingPlay\.image/);
});

test('the RadioBrowser browser sends the station logo with the play request', () => {
    assert.match(radioBrowser, /image: item\.imageUrl/);
    assert.match(radioBrowser, /containerArt: pendingPlay\.image/);
});
