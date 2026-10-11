import assert from 'node:assert/strict';
import test from 'node:test';

import { canonicalLocation, sameContentLocation } from '../static/js/contentLocation.mjs';
import { artworkFor, presetArtIndex } from '../static/js/recentsArt.mjs';

const relative = '/station?data=eyJuYW1lIjoiRG9jIFJhZGlvIn0%3D';
const absolute = 'http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJuYW1lIjoiRG9jIFJhZGlvIn0%3D';
const boseCloud = 'https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJuYW1lIjoiRG9jIFJhZGlvIn0%3D';

test('an Orion station is canonicalised to its relative location', () => {
    assert.equal(canonicalLocation('LOCAL_INTERNET_RADIO', absolute), relative);
    assert.equal(canonicalLocation('LOCAL_INTERNET_RADIO', boseCloud), relative);
    assert.equal(canonicalLocation('LOCAL_INTERNET_RADIO', relative), relative);
    assert.ok(sameContentLocation('LOCAL_INTERNET_RADIO', absolute, relative));
});

test('other locations and other sources stay as they are', () => {
    assert.equal(canonicalLocation('TUNEIN', absolute), absolute);
    assert.equal(canonicalLocation('RADIO_BROWSER', '/stations/byuuid/1'), '/stations/byuuid/1');
    assert.equal(canonicalLocation('LOCAL_INTERNET_RADIO', 'http://stream.example.org/live.mp3'), 'http://stream.example.org/live.mp3');
    assert.equal(canonicalLocation('LOCAL_INTERNET_RADIO', undefined), '');
    assert.ok(!sameContentLocation('LOCAL_INTERNET_RADIO', relative, '/station?data=other'));
});

test('a relative recent borrows art from an absolute preset of the same station', () => {
    const index = presetArtIndex({
        Preset: [{
            ID: 1,
            ContentItem: {
                Source: 'LOCAL_INTERNET_RADIO',
                SourceAccount: '',
                Location: absolute,
                ContainerArt: 'http://example.invalid/doc.png',
            },
        }],
    });

    const recent = {
        Source: 'LOCAL_INTERNET_RADIO',
        SourceAccount: 'LOCAL_INTERNET_RADIO',
        Location: relative,
    };

    assert.equal(artworkFor(recent, index), 'http://example.invalid/doc.png');
});

function orion(name, imageUrl, streamUrl) {
    const data = Buffer.from(JSON.stringify({ name, imageUrl, streamUrl })).toString('base64');
    return '/station?data=' + encodeURIComponent(data);
}

test('all spellings of one stream are one station (issue 784)', () => {
    const stream = 'https://stream.example.org/technikum.mp3';
    const rel = orion('TechnikumCity', '', stream);
    const spellings = [
        orion('Technikum City', 'https://img.example.org/logo.png', stream),
        'http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion' + rel,
        'http://192.0.2.10:8000/custom/v1/playback/' + Buffer.from(stream).toString('base64') + '?name=TechnikumCity',
        'http://192.0.2.10:8000/custom/v1/playback/' + Buffer.from(stream).toString('base64url'),
        'http://192.0.2.10:8000/custom/v1/playback/' + encodeURIComponent(stream),
    ];
    for (const location of spellings) {
        assert.ok(sameContentLocation('LOCAL_INTERNET_RADIO', rel, location), location);
    }
    assert.equal(canonicalLocation('LOCAL_INTERNET_RADIO', rel), 'stream:' + stream);
    assert.ok(!sameContentLocation('LOCAL_INTERNET_RADIO', rel, orion('TechnikumCity', '', 'https://stream.example.org/other.mp3')));
    assert.ok(!sameContentLocation('LOCAL_INTERNET_RADIO', rel, stream));
});

test('the service and the player agree on the stream key', () => {
    // Same input as TestCanonicalContentLocationKeysStationsByStream's
    // playback adapter case on the Go side.
    const stream = 'https://stream.example.org/technikum.mp3';
    const adapter = 'http://192.0.2.10:8000/custom/v1/playback/' + Buffer.from(stream).toString('base64') + '?name=TechnikumCity';
    assert.equal(canonicalLocation('LOCAL_INTERNET_RADIO', adapter), 'stream:' + stream);
});
