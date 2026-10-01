import assert from 'node:assert/strict';
import test from 'node:test';

import {
    createLatestWinsScheduler,
    shouldSurfaceLatestFinal,
} from '../static/js/latestWinsScheduler.mjs';

function deferred() {
    let resolve;
    let reject;
    const promise = new Promise((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
}

function fakeClock() {
    let time = 0;
    let nextId = 1;
    const timers = new Map();

    return {
        now: () => time,
        setTimer(callback, delay) {
            const id = nextId++;
            timers.set(id, { callback, at: time + delay });
            return id;
        },
        clearTimer: id => timers.delete(id),
        advance(milliseconds) {
            const target = time + milliseconds;
            while (true) {
                const due = [...timers.entries()]
                    .filter(([, timer]) => timer.at <= target)
                    .sort((a, b) => a[1].at - b[1].at || a[0] - b[0])[0];
                if (!due) break;
                const [id, timer] = due;
                timers.delete(id);
                time = timer.at;
                timer.callback();
            }
            time = target;
        },
    };
}

async function flushPromises() {
    await Promise.resolve();
    await Promise.resolve();
}

test('only the latest forced-final outcome is visible as an error', () => {
    assert.equal(shouldSurfaceLatestFinal({ isLatest: true, final: true }), true);
    assert.equal(shouldSurfaceLatestFinal({ isLatest: true, final: false }), false);
    assert.equal(shouldSurfaceLatestFinal({ isLatest: false, final: true }), false);
});

test('allows one in flight, keeps only the latest pending, and spaces starts by 200ms', async () => {
    const clock = fakeClock();
    const requests = [];
    const scheduler = createLatestWinsScheduler({
        send(value) {
            const request = deferred();
            requests.push({ value, startedAt: clock.now(), request });
            return request.promise;
        },
        now: clock.now,
        setTimer: clock.setTimer,
        clearTimer: clock.clearTimer,
    });

    scheduler.queue(10);
    scheduler.queue(20);
    scheduler.queue(30);
    clock.advance(100);
    assert.deepEqual(requests.map(({ value }) => value), [10]);

    requests[0].request.resolve({ success: true });
    await flushPromises();
    clock.advance(99);
    assert.equal(requests.length, 1);
    clock.advance(1);
    assert.deepEqual(requests.map(({ value, startedAt }) => [value, startedAt]), [
        [10, 0],
        [30, 200],
    ]);
});

test('forced final promotes an in-flight value without relabelling its projection fence', async () => {
    const request = deferred();
    const results = [];
    let sends = 0;
    const scheduler = createLatestWinsScheduler({
        send() {
            sends += 1;
            return request.promise;
        },
        onResult(_response, metadata) {
            results.push([
                metadata.final,
                metadata.isLatest,
                metadata.projectionKey,
                metadata.targetKey,
            ]);
        },
    });

    scheduler.queue(40, {
        interactionGeneration: 3,
        projectionKey: 'projection-0',
        targetKey: 'target-0',
    });
    scheduler.queue(40, {
        force: true,
        interactionGeneration: 3,
        projectionKey: 'projection-1',
        targetKey: 'target-1',
    });
    request.resolve({ success: true });
    await flushPromises();

    assert.equal(sends, 1);
    assert.deepEqual(results, [[true, true, 'projection-0', 'target-0']]);
});

test('forced final promotes the latest pending value without relabelling its projection fence', async () => {
    const clock = fakeClock();
    const requests = [];
    const results = [];
    const scheduler = createLatestWinsScheduler({
        send(value) {
            const request = deferred();
            requests.push({ value, request });
            return request.promise;
        },
        now: clock.now,
        setTimer: clock.setTimer,
        clearTimer: clock.clearTimer,
        onResult(_response, metadata) {
            results.push([
                metadata.value,
                metadata.final,
                metadata.isLatest,
                metadata.projectionKey,
                metadata.targetKey,
            ]);
        },
    });

    scheduler.queue(10, {
        interactionGeneration: 3,
        projectionKey: 'projection-0',
        targetKey: 'target-0',
    });
    scheduler.queue(40, {
        interactionGeneration: 3,
        projectionKey: 'projection-1',
        targetKey: 'target-1',
    });
    scheduler.queue(40, {
        force: true,
        interactionGeneration: 3,
        projectionKey: 'projection-2',
        targetKey: 'target-2',
    });
    requests[0].request.resolve({ success: true });
    await flushPromises();
    clock.advance(200);
    requests[1].request.resolve({ success: true });
    await flushPromises();

    assert.deepEqual(requests.map(({ value }) => value), [10, 40]);
    assert.deepEqual(results, [
        [10, false, false, 'projection-0', 'target-0'],
        [40, true, true, 'projection-1', 'target-1'],
    ]);
});

test('forced final reuses a settled result without relabelling its projection fence', async () => {
    const request = deferred();
    const results = [];
    let sends = 0;
    const scheduler = createLatestWinsScheduler({
        send() {
            sends += 1;
            return request.promise;
        },
        onResult(_response, metadata) {
            results.push([metadata.final, metadata.projectionKey, metadata.targetKey]);
        },
    });

    scheduler.queue(40, {
        interactionGeneration: 3,
        projectionKey: 'projection-0',
        targetKey: 'target-0',
    });
    request.resolve({ success: true });
    await flushPromises();
    scheduler.queue(40, {
        force: true,
        interactionGeneration: 3,
        projectionKey: 'projection-1',
        targetKey: 'target-1',
    });

    assert.equal(sends, 1);
    assert.deepEqual(results, [
        [false, 'projection-0', 'target-0'],
        [true, 'projection-0', 'target-0'],
    ]);
});

test('a new interaction retries the same finalized value', async () => {
    const clock = fakeClock();
    const requests = [];
    const scheduler = createLatestWinsScheduler({
        send(value) {
            const request = deferred();
            requests.push({ value, request });
            return request.promise;
        },
        now: clock.now,
        setTimer: clock.setTimer,
        clearTimer: clock.clearTimer,
    });

    scheduler.queue(40, { interactionGeneration: 3 });
    requests[0].request.resolve({ success: true });
    await flushPromises();
    scheduler.queue(40, { force: true, interactionGeneration: 3 });
    scheduler.queue(40, { interactionGeneration: 4 });
    clock.advance(200);

    assert.deepEqual(requests.map(({ value }) => value), [40, 40]);
});

test('a failed finalized value remains retryable in a new interaction', async () => {
    const clock = fakeClock();
    const requests = [];
    const errors = [];
    const scheduler = createLatestWinsScheduler({
        send(value) {
            const request = deferred();
            requests.push({ value, request });
            return request.promise;
        },
        now: clock.now,
        setTimer: clock.setTimer,
        clearTimer: clock.clearTimer,
        onError(error, metadata) {
            errors.push([error.message, metadata.interactionGeneration, metadata.final]);
        },
    });

    scheduler.queue(40, { interactionGeneration: 3 });
    requests[0].request.reject(new Error('offline'));
    await flushPromises();
    scheduler.queue(40, { force: true, interactionGeneration: 3 });
    scheduler.queue(40, { interactionGeneration: 4 });
    clock.advance(200);

    assert.deepEqual(requests.map(({ value }) => value), [40, 40]);
    assert.deepEqual(errors, [
        ['offline', 3, false],
        ['offline', 3, true],
    ]);
});

test('marks an older response non-latest when a newer preview is pending', async () => {
    const requests = [];
    const results = [];
    const scheduler = createLatestWinsScheduler({
        interval: 0,
        send(value) {
            const request = deferred();
            requests.push({ value, request });
            return request.promise;
        },
        onResult(_response, metadata) {
            results.push([metadata.value, metadata.isLatest]);
        },
    });

    scheduler.queue(25, { interactionGeneration: 1 });
    scheduler.queue(75, { interactionGeneration: 1 });
    requests[0].request.resolve({ success: true });
    await flushPromises();

    assert.deepEqual(results, [[25, false]]);
    assert.deepEqual(requests.map(({ value }) => value), [25, 75]);
});
