import assert from 'node:assert/strict';
import test from 'node:test';

import { api } from '../static/js/api.js';

test('selectSource posts the exact source and account body', async () => {
    const requests = [];
    globalThis.fetch = async (url, options) => {
        requests.push({ url, options });
        return new Response('{"success":true}', {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
        });
    };

    await api.selectSource('speaker', 'AUX', 'AUX1');
    await api.selectSource('speaker', 'PRODUCT');

    const request = requests[0];
    assert.equal(request.url, '/api/control/devices/speaker/action/source');
    assert.equal(request.options.method, 'POST');
    assert.deepEqual(JSON.parse(request.options.body), { source: 'AUX', account: 'AUX1' });
    assert.deepEqual(JSON.parse(requests[1].options.body), { source: 'PRODUCT', account: '' });
});

test('source selection rejects non-2xx responses', async () => {
    globalThis.fetch = async () => new Response('{"success":false,"error":"offline"}', {
        status: 503,
        headers: { 'Content-Type': 'application/json' },
    });

    await assert.rejects(api.selectSource('speaker', 'AUX', 'AUX1'), /offline/);
});

test('source selection rejects application failures on 2xx responses', async () => {
    globalThis.fetch = async () => new Response('{"success":false,"error":"rejected"}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
    });

    await assert.rejects(api.selectSource('speaker', 'AUX', 'AUX1'), /rejected/);
});

test('legacy API consumers retain response-level error handling', async () => {
    globalThis.fetch = async () => new Response('{"success":false,"error":"offline"}', {
        status: 503,
        headers: { 'Content-Type': 'application/json' },
    });

    assert.deepEqual(await api.devices(), { success: false, error: 'offline' });
});
