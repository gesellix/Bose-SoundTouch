import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const presets = await readFile(new URL('../static/js/components/Presets.js', import.meta.url), 'utf8');
const css = await readFile(new URL('../static/css/app.css', import.meta.url), 'utf8');

// Regression guard for issue #646: the save-to-preset button used to be
// rendered only while something was playing *and* revealed only on hover, so
// two reporters concluded the feature did not exist and filed a bug against
// something else entirely.
test('the save button is disabled rather than hidden when nothing is playing', () => {
    assert.match(presets, /disabled=\$\{!canSave\}/);
    assert.doesNotMatch(presets, /\$\{canSave && html/);
});

// Issue 700: a STORED_MUSIC preset tile showed the raw source name and had no
// accent colour, leaving it the only tile rendered with the default border.
// RADIO_BROWSER had the same label gap.
test('library and radio-browser presets get a readable label', () => {
    assert.match(presets, /STORED_MUSIC:\s*'Library'/);
    assert.match(presets, /RADIO_BROWSER:\s*'Radio Browser'/);
});

test('a library preset tile has its own accent colour', () => {
    assert.match(css, /\[data-source="STORED_MUSIC"\][^{]*\{[^}]*--slot-color/);
});

test('the save button is not hidden behind a hover reveal', () => {
    const rule = css.match(/\.preset-save-btn \{[^}]*\}/);
    assert.ok(rule, 'expected a .preset-save-btn rule');
    // Touch devices have no hover, so opacity:0 made it unreachable there.
    assert.doesNotMatch(rule[0], /opacity:\s*0\s*[;}]/);
    assert.doesNotMatch(css, /:hover\s+\.preset-save-btn/);
});

test('an empty slot saves the current content when clicked', () => {
    assert.match(presets, /savableEmpty = isEmpty && canSave/);
    assert.match(presets, /if \(savableEmpty\) store\(\)/);
});

// The grid item is .preset-slot-wrap, which stretches to the row height; the
// tile inside it needs to be told to fill that, or a row of one- and two-line
// preset names ends up with ragged bottom edges.
test('preset tiles fill their grid cell so a row lines up', () => {
    assert.match(css, /\.preset-slot-wrap \.preset-slot \{[^}]*height:\s*100%/);
});
