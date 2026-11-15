import assert from 'node:assert/strict';
import test from 'node:test';

import { createLogsController, formatLogLines } from '../web/js/logsState.mjs';

const entry = (message, time = '2026-10-26T12:00:00Z') => ({ message, time });

const flush = async () => { await Promise.resolve(); await Promise.resolve(); };

test('logs keep the latest entries, dropped gap and safe plain-text formatting', async () => {
    const log = createLogsController({
        maxEntries: 2,
        now: () => '12:34',
        request: async () => ({ entries: [entry('<img src=x onerror=alert(1)>'), entry('INFO'), entry('error')], nextSince: 3, dropped: 2 }),
        schedule: () => 1,
        unschedule: () => {},
    });
    log.start();
    await flush();
    assert.deepEqual(log.snapshot().entries.map(line => line.message), ['INFO', 'error']);
    assert.equal(log.snapshot().nextSince, 3);
    assert.equal(log.snapshot().droppedTotal, 2);
    assert.equal(log.snapshot().status, '2 buffered · 2 dropped · last update 12:34');
    assert.equal(formatLogLines(log.snapshot().entries, ' ERROR '), '2026-10-26 12:00:00  error');
    log.stop();
});

test('only an open visible tab polls; stale responses cannot replace a new snapshot', async () => {
    let resolveFirst;
    const calls = [];
    const timers = [];
    let hidden = false;
    const log = createLogsController({
        request: (since, signal) => {
            calls.push({ since, signal });
            if (calls.length === 1) return new Promise(resolve => { resolveFirst = resolve; });
            return Promise.resolve({ entries: [entry('new')], nextSince: 9, dropped: 0 });
        },
        isHidden: () => hidden,
        schedule: callback => { timers.push(callback); return timers.length; },
        unschedule: () => {},
    });
    log.start();
    timers[0]();
    assert.equal(calls.length, 1, 'overlapping tick must not start a second request');
    assert.equal(log.start(), false, 'an already active tab must not reset or duplicate its timer');
    log.stop();
    assert.equal(calls[0].signal.aborted, true);
    log.start();
    await flush();
    assert.deepEqual(log.snapshot().entries.map(line => line.message), ['new']);
    resolveFirst({ entries: [entry('stale')], nextSince: 99, dropped: 2 });
    await flush();
    assert.deepEqual(log.snapshot().entries.map(line => line.message), ['new']);
    assert.equal(log.snapshot().nextSince, 9);
    hidden = true;
    timers[1]();
    assert.equal(calls.length, 2);
    log.stop();
    timers[1]();
    assert.equal(calls.length, 2);
});

test('filter and follow settings survive reopen while entries reset', async () => {
    const log = createLogsController({
        request: async () => ({ entries: [entry('first')], nextSince: 1, dropped: 0 }),
        schedule: () => 1,
        unschedule: () => {},
    });
    log.setFilter(' FIRST ');
    log.setFollowTail(false);
    log.start();
    await flush();
    assert.equal(formatLogLines(log.snapshot().entries, log.snapshot().filter), '2026-10-26 12:00:00  first');
    log.stop();
    log.start();
    assert.equal(log.snapshot().filter, ' FIRST ');
    assert.equal(log.snapshot().followTail, false);
    assert.equal(log.snapshot().nextSince, 0);
    log.stop();
});
