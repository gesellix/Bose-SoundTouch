import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const components = new URL('../static/js/components/', import.meta.url);

test('logical zone components contain only the volume-free presentation surface', async () => {
    const [deviceList, zone] = await Promise.all([
        readFile(new URL('DeviceList.js', components), 'utf8'),
        readFile(new URL('Zone.js', components), 'utf8'),
    ]);

    assert.match(deviceList, /zone-card/);
    assert.match(deviceList, /healthLabel/);
    assert.match(deviceList, /onSelect\(controlID\)/);
    assert.match(zone, /<details class="zone-member-details">/);
    assert.match(zone, /member\.physicalMembers/);
    assert.match(zone, /zone-physical-role/);

    for (const source of [deviceList, zone]) {
        assert.doesNotMatch(source, /ZoneMemberVolumeControl|zoneVolume|MemberSettings|balance/i);
        assert.doesNotMatch(source, /type="range"/);
    }
});
