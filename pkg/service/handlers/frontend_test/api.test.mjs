import assert from 'node:assert/strict';
import test from 'node:test';

import { adminApi } from '../web/js/api.js';

test('logs API keeps the existing setup route and cursor', async () => {
    const original = globalThis.fetch;
    const calls = [];
    globalThis.fetch = async (url, options) => {
        calls.push({ url, options });
        return new Response('{"entries":[],"nextSince":42,"dropped":0,"capacity":5000}');
    };
    try {
        const controller = new AbortController();
        const result = await adminApi.logs(42, controller.signal);
        assert.equal(result.nextSince, 42);
        assert.equal(calls[0].url, '/api/setup/logs?since=42');
        assert.equal(calls[0].options.signal, controller.signal);
    } finally {
        globalThis.fetch = original;
    }
});

test('logs API reports an HTTP auth failure without rendering its body', async () => {
    const original = globalThis.fetch;
    globalThis.fetch = async () => new Response('<script>secret</script>', { status: 401 });
    try {
        await assert.rejects(adminApi.logs(0), /HTTP 401/);
    } finally {
        globalThis.fetch = original;
    }
});
