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

// A 4xx is produced before any speaker call (unknown device, unparseable
// body, empty source), so the command provably never went out.
test('source selection reports a 4xx as definitive', async () => {
    globalThis.fetch = async () => new Response('{"success":false,"error":"Device not found"}', {
        status: 404,
        headers: { 'Content-Type': 'application/json' },
    });

    await assert.rejects(api.selectSource('speaker', 'AUX', 'AUX1'), error => {
        assert.match(error.message, /Device not found/);
        assert.equal(error.definitive, true);
        return true;
    });
});

// A 5xx is how a failed Client.SelectSource surfaces, and a request that timed
// out after the speaker already switched is indistinguishable from one it never
// received. The caller must keep verifying by readback.
test('source selection reports a 5xx as non-definitive', async () => {
    globalThis.fetch = async () => new Response('{"success":false,"error":"speaker unreachable"}', {
        status: 500,
        headers: { 'Content-Type': 'application/json' },
    });

    await assert.rejects(api.selectSource('speaker', 'AUX', 'AUX1'), error => {
        assert.match(error.message, /speaker unreachable/);
        assert.equal(error.definitive, false);
        return true;
    });
});

test('source selection reports an unreadable body as non-definitive on 5xx', async () => {
    globalThis.fetch = async () => new Response('gateway timeout', { status: 504 });

    await assert.rejects(api.selectSource('speaker', 'AUX', 'AUX1'), error => {
        assert.equal(error.definitive, false);
        return true;
    });
});

test('legacy API consumers retain response-level error handling', async () => {
    globalThis.fetch = async () => new Response('{"success":false,"error":"offline"}', {
        status: 503,
        headers: { 'Content-Type': 'application/json' },
    });

    assert.deepEqual(await api.devices(), { success: false, error: 'offline' });
});
