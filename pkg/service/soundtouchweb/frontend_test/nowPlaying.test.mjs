import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const nowPlaying = await readFile(
    new URL('../static/js/components/NowPlaying.js', import.meta.url), 'utf8');

// Radio stations carry their logo on the ContentItem rather than in the <art>
// element, so the now-playing card showed nothing for a TuneIn station whose
// preset tile displayed the same logo fine. The Go model already resolves both
// in this order (NowPlaying.GetArtworkURL); the frontend has to match it.
test('the now-playing card falls back to the ContentItem artwork', () => {
    assert.match(nowPlaying, /nowPlaying\.Art\?\.URL \|\| nowPlaying\.ContentItem\?\.ContainerArt/);
});

// The star used to say only "Saved as preset", leaving you to hunt for which
// slot it meant.
test('the preset star names the slot it is saved in', () => {
    assert.match(nowPlaying, /Saved as preset \$\{mappedPreset\.ID\}/);
});
