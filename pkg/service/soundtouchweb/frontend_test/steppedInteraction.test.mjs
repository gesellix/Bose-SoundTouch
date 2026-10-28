import assert from 'node:assert/strict';
import test from 'node:test';
import { createSteppedInteraction, nearestStep } from '../static/js/steppedInteraction.mjs';

function deferred() {
    let resolve;
    let reject;
    const promise = new Promise((done, fail) => { resolve = done; reject = fail; });
    return { promise, resolve, reject };
}
const target = (extra = {}) => ({ key: 'physical-A', projection: '["bass",0]',
    value: 0, min: -9, max: 0, disabled: false, ...extra });
const flush = async () => { await Promise.resolve(); await Promise.resolve(); };

test('availability loss retains a dispatched final failure and blocks new writes', async () => {
    for (const rejected of [false, true]) {
        const pending = deferred();
        const writes = [];
        const state = createSteppedInteraction({ interval: 0, send: (_, value) => {
            writes.push(value);
            return pending.promise;
        } });
        state.update(target());
        state.choose(-1);
        state.update(target({ disabled: true, revision: 2 }));
        assert.equal(state.view().busy, true);
        assert.equal(state.view().value, -1);
        state.choose(-2);
        if (rejected) pending.reject(new Error('Final transport failure'));
        else pending.resolve({ success: false, error: 'Final rejected write' });
        await flush();
        const failure = state.view().failure;
        assert.match(failure, /Final/);
        state.update(target({ disabled: true, revision: 3 }));
        state.update(target({ revision: 4 }));
        assert.equal(state.view().failure, failure);
        assert.deepEqual(writes, [-1]);
        state.dispose();
    }
});

test('same-target unavailable acoustic changes cannot erase an existing final error', async () => {
    const state = createSteppedInteraction({ interval: 0, send: () => ({ success: false, error: 'Final error' }) });
    state.update(target());
    state.choose(-1);
    await flush();
    state.update(target({ disabled: true, value: -2, projection: '["bass",-2]' }));
    assert.equal(state.view().failure, 'Final error');
    state.update(target({ disabled: true, value: -3, projection: '["bass",-3]' }));
    assert.equal(state.view().failure, 'Final error');
    state.update(target({ value: -3, projection: '["bass",-3]' }));
    assert.equal(state.view().failure, 'Final error');
    state.update(target({ value: -4, projection: '["bass",-4]' }));
    assert.equal(state.view().failure, '');
    state.dispose();
});

test('bounds and aggregate volume revision changes preserve outcome identity', async () => {
    const pending = deferred();
    const state = createSteppedInteraction({ interval: 0, send: () => pending.promise });
    state.update(target());
    state.choose(-1);
    state.update(target({ min: -5, max: 0, defaultValue: -1, revision: 10, volume: 50 }));
    pending.resolve({ success: false, error: 'Final failure despite bounds refresh' });
    await flush();
    state.update(target({ min: -5, max: 0, defaultValue: -1, revision: 11, volume: 60 }));
    assert.equal(state.view().failure, 'Final failure despite bounds refresh');
    state.dispose();
});

test('queued clicks are checked against current availability and bounds before dispatch', async () => {
    for (const unavailable of [false, true]) {
        const first = deferred();
        const writes = [];
        const state = createSteppedInteraction({ interval: 0, send: (_, value) => {
            writes.push(value);
            return first.promise;
        } });
        state.update(target());
        state.choose(-1);
        state.choose(-2);
        state.update(target(unavailable ? { disabled: true } : { min: -1 }));
        first.resolve({ success: true, confirmedValue: -1 });
        await flush();
        await flush();
        assert.deepEqual(writes, [-1]);
        if (unavailable) {
            assert.equal(state.view().failure, '');
            assert.equal(state.view().value, -1);
            assert.equal(state.view().confirmedValue, -1);
        } else assert.match(state.view().failure, /range changed/);
        state.dispose();
    }
});

test('tap positions round to the nearest valid integer and clamp to reported bounds', () => {
    assert.equal(nearestStep(100, 100, 90, -9, 0), -9);
    assert.equal(nearestStep(114, 100, 90, -9, 0), -8);
    assert.equal(nearestStep(115, 100, 90, -9, 0), -7);
    assert.equal(nearestStep(190, 100, 90, -9, 0), 0);
    assert.equal(nearestStep(-100, 100, 90, -9, 0), -9);
    assert.equal(nearestStep(1000, 100, 90, -9, 0), 0);
    assert.equal(nearestStep(150, 100, 100, -10, 10), 0);
    assert.equal(nearestStep(150, 100, 100, 0, 0), 0);
    for (const args of [[100, 100, 0, -9, 0], [NaN, 100, 90, -9, 0],
        [150, 100, 90, 0, -9], [150, 100, 90, -9.5, 0]]) {
        assert.equal(nearestStep(...args), null);
    }
});

const rect = { left: 100, top: 20, width: 90, height: 52 };
const pointer = (extra = {}) => ({ pointerId: 1, isPrimary: true, button: 0,
    clientX: 130, clientY: 40, ...extra });

test('a tap commits its original nearest step once, without writes during pointer movement', () => {
    const writes = [];
    const state = createSteppedInteraction({ send: (_, value) => { writes.push(value); return new Promise(() => {}); } });
    state.update(target());
    state.beginTap(pointer({ clientX: 134 }));
    state.moveTap(pointer({ clientX: 138 }));
    assert.deepEqual(writes, []);
    state.endTap(pointer({ clientX: 138 }), rect);
    // The tiny release movement crosses a rounding boundary but does not move the choice.
    assert.deepEqual(writes, [-6]);
    state.endTap(pointer({ clientX: 138 }), rect);
    assert.deepEqual(writes, [-6]);
    state.dispose();
});

test('drag, cancel, blur, unchanged, disabled and non-primary pointers cause zero writes', () => {
    const writes = [];
    const state = createSteppedInteraction({ send: (_, value) => writes.push(value) });
    state.update(target());
    state.beginTap(pointer());
    state.moveTap(pointer({ clientX: 160 }));
    state.endTap(pointer(), rect);
    state.beginTap(pointer());
    state.endTap(pointer({ clientX: 160 }), rect);
    for (let cancellation = 0; cancellation < 2; cancellation += 1) {
        state.beginTap(pointer());
        state.cancelTap();
        state.endTap(pointer(), rect);
    }
    state.beginTap(pointer({ clientX: 190 }));
    state.endTap(pointer({ clientX: 190 }), rect);
    state.beginTap(pointer());
    state.endTap(pointer({ clientY: 100 }), rect);
    state.beginTap(pointer({ button: 2 }));
    state.endTap(pointer(), rect);
    state.beginTap(pointer({ isPrimary: false }));
    state.endTap(pointer(), rect);
    state.update(target({ disabled: true }));
    state.beginTap(pointer());
    state.endTap(pointer(), rect);
    state.choose(-1);
    assert.equal(state.keyDown('Home'), false);
    assert.deepEqual(writes, []);
    state.dispose();
});

test('tap ownership is cancelled when its target or availability changes', () => {
    for (const replacement of [target({ key: 'physical-B' }), target({ disabled: true }),
        target({ key: '["physical-A",2]', epoch: 2 })]) {
        const writes = [];
        const state = createSteppedInteraction({ send: (_, value) => writes.push(value) });
        state.update(target());
        state.beginTap(pointer());
        state.update(replacement);
        state.update(target());
        state.endTap(pointer(), rect);
        assert.deepEqual(writes, []);
        state.dispose();
    }
});

test('the first choice starts immediately and only the newest pending choice starts after settlement', async () => {
    const requests = [];
    const state = createSteppedInteraction({
        now: () => 0,
        setTimer: () => { assert.fail('Discrete choices must not wait for a timer'); },
        send: (_, value) => {
            const request = deferred();
            requests.push({ value, request });
            return request.promise;
        },
    });
    state.update(target());
    state.choose(-1);
    state.choose(-4);
    state.choose(-7);
    assert.deepEqual(requests.map(request => request.value), [-1]);
    assert.equal(state.view().value, -7);
    requests[0].request.resolve({ success: true, confirmedValue: -1 });
    await flush();
    assert.equal(state.view().value, -7);
    assert.deepEqual(requests.map(request => request.value), [-1, -7]);
    requests[1].request.resolve({ success: true, confirmedValue: -7 });
    await flush();
    assert.equal(state.view().confirmedValue, -7);
    assert.equal(state.view().busy, false);
    state.dispose();
});

test('synchronous +/- and keyboard choices accumulate from the local requested value', async () => {
    const first = deferred();
    const last = deferred();
    const writes = [];
    const state = createSteppedInteraction({ send: (_, value) => {
        writes.push(value);
        return writes.length === 1 ? first.promise : last.promise;
    } });
    state.update(target({ value: -4, projection: '["bass",-4]' }));
    state.step(-1);
    state.step(-1);
    state.step(1);
    assert.equal(state.view().value, -5);
    assert.equal(state.keyDown('ArrowLeft'), true);
    assert.equal(state.keyDown('ArrowDown'), true);
    assert.equal(state.view().value, -7);
    assert.equal(state.keyDown('ArrowRight'), true);
    assert.equal(state.keyDown('ArrowUp'), true);
    assert.equal(state.view().value, -5);
    assert.equal(state.keyDown('Home'), true);
    state.step(-1);
    assert.equal(state.view().value, -9);
    assert.equal(state.keyDown('End'), true);
    state.step(1);
    assert.equal(state.view().value, 0);
    assert.equal(state.keyDown('Tab'), false);
    assert.deepEqual(writes, [-5]);
    first.resolve({ success: false, error: 'Older intermediate failure' });
    await flush();
    assert.deepEqual(writes, [-5, 0]);
    assert.equal(state.view().value, 0);
    assert.equal(state.view().failure, '');
    last.resolve({ success: true, confirmedValue: 0 });
    await flush();
    assert.equal(state.view().confirmedValue, 0);
    state.dispose();
});

test('a matching newer acoustic observation confirms unverified readback but never masks a definitive failure', async () => {
    for (const unverified of [true, false]) {
        const pending = deferred();
        const state = createSteppedInteraction({ send: () => pending.promise });
        state.update(target());
        state.choose(-2);
        state.update(target({ value: -2, projection: '["bass",-2]' }));
        pending.resolve({ success: false, outcome: unverified ? 'unverified' : 'failed', error: 'Readback unavailable' });
        await flush();
        assert.equal(state.view().failure, unverified ? '' : 'Readback unavailable');
        assert.equal(state.view().confirmedValue, unverified ? -2 : null);
        assert.equal(state.view().value, -2);
        state.dispose();
    }
});

test('unverified clears only on a changed valid matching observation, not unrelated or nonmatching updates', async () => {
    const pending = deferred();
    const state = createSteppedInteraction({ send: () => pending.promise });
    state.update(target());
    state.choose(-2);
    state.update(target({ value: -1, projection: '["bass",-1]' }));
    pending.resolve({ success: false, outcome: 'unverified', error: 'Write could not be verified' });
    await flush();
    const warning = state.view().failure;
    assert.equal(warning, 'Write could not be verified');
    state.update(target({ value: -1, projection: '["bass",-1]', revision: 10, volume: 40 }));
    assert.equal(state.view().failure, warning);
    state.update(target({ value: -3, projection: '["bass",-3]' }));
    assert.equal(state.view().failure, warning);
    state.update(target({ value: -2, projection: '["bass",-2]', disabled: true }));
    state.update(target({ value: -2, projection: '["bass",-2]' }));
    assert.equal(state.view().failure, warning);
    state.update(target({ value: -2, projection: '["bass",invalid]', min: -1 }));
    assert.equal(state.view().failure, warning);
    state.update(target({ value: -2, projection: '["bass",-2]' }));
    assert.equal(state.view().failure, '');
    assert.equal(state.view().confirmedValue, -2);
    state.dispose();
});

test('disable and reconnect discard queued choices without replaying them on the same identity', async () => {
    const first = deferred();
    const writes = [];
    const state = createSteppedInteraction({ send: (_, value) => {
        writes.push(value);
        return first.promise;
    } });
    state.update(target());
    state.choose(-1);
    state.choose(-2);
    state.update(target({ disabled: true }));
    state.update(target());
    first.resolve({ success: true, confirmedValue: -1 });
    await flush();
    await flush();
    assert.deepEqual(writes, [-1]);
    assert.equal(state.view().value, -1);
    assert.equal(state.view().confirmedValue, -1);
    assert.equal(state.view().busy, false);
    assert.equal(state.view().failure, '');
    state.dispose();
});

test('cancelled pending admission retains the last sent failure, unverified warning or confirmation', async () => {
    for (const response of [
        { success: false, error: 'Speaker rejected sent write' },
        { success: false, outcome: 'unverified', error: 'Sent write could not be verified' },
        { success: true, confirmedValue: -1 },
        new Error('Sent write transport failed'),
    ]) {
        for (const reconnect of [false, true]) {
            const first = deferred();
            const writes = [];
            const state = createSteppedInteraction({ send: (_, value) => {
                writes.push(value);
                return first.promise;
            } });
            state.update(target());
            state.choose(-1);
            state.choose(-2);
            state.update(target({ disabled: true }));
            if (reconnect) state.update(target());
            if (response instanceof Error) first.reject(response);
            else first.resolve(response);
            await flush();
            await flush();
            assert.deepEqual(writes, [-1]);
            assert.equal(state.view().failure, response instanceof Error ? response.message : response.error || '');
            assert.equal(state.view().confirmedValue, response.success ? -1 : null);
            assert.equal(state.view().value, response.success ? -1 : 0);
            assert.equal(state.view().busy, false);
            state.update(target({ revision: 2, volume: 30, disabled: !reconnect }));
            assert.equal(state.view().failure, response instanceof Error ? response.message : response.error || '');
            assert.equal(state.view().confirmedValue, response.success ? -1 : null);
            assert.deepEqual(writes, [-1]);
            if (response.outcome === 'unverified' && reconnect) {
                state.update(target({ value: -2, projection: '["bass",-2]' }));
                assert.equal(state.view().failure, response.error);
                state.update(target({ value: -1, projection: '["bass",-1]' }));
                assert.equal(state.view().failure, '');
                assert.equal(state.view().confirmedValue, -1);
            }
            state.dispose();
        }
    }
});

test('cancelled pending admission never inherits a sent outcome from a previous owner or epoch', async () => {
    for (const replacement of [target({ key: 'physical-B' }), target({ key: '["physical-A",2]', epoch: 2 })]) {
        for (const response of [
            { success: false, error: 'Old owner failed' },
            { success: false, outcome: 'unverified', error: 'Old owner unverified' },
            { success: true, confirmedValue: -1 },
            new Error('Old owner transport failed'),
        ]) {
            const first = deferred();
            const writes = [];
            const state = createSteppedInteraction({ send: (_, value) => {
                writes.push(value);
                return first.promise;
            } });
            state.update(target());
            state.choose(-1);
            state.choose(-2);
            state.update(replacement);
            state.choose(-3);
            state.update({ ...replacement, disabled: true });
            if (response instanceof Error) first.reject(response);
            else first.resolve(response);
            await flush();
            await flush();
            assert.deepEqual(writes, [-1]);
            assert.equal(state.view().failure, '');
            assert.equal(state.view().confirmedValue, null);
            assert.equal(state.view().value, 0);
            assert.equal(state.view().busy, false);
            state.dispose();
        }
    }
});

test('an owner change after the sent outcome is remembered cannot restore it when the skipped choice settles', async () => {
    for (const response of [
        { success: false, error: 'Remembered old A failure' },
        { success: false, outcome: 'unverified', error: 'Remembered old A warning' },
        { success: true, confirmedValue: -1 },
    ]) {
        const first = deferred();
        const writes = [];
        const state = createSteppedInteraction({ send: (_, value) => {
            writes.push(value);
            return first.promise;
        } });
        state.update(target());
        state.choose(-1);
        state.choose(-2);
        state.update(target({ disabled: true }));
        first.resolve(response);
        // Let the sent result settle and the skipped request start, but change
        // ownership before the skipped request's completion microtask runs.
        await Promise.resolve();
        assert.equal(state.view().busy, true);
        state.update(target({ key: 'physical-B' }));
        state.update(target());
        await flush();
        assert.deepEqual(writes, [-1]);
        assert.equal(state.view().failure, '');
        assert.equal(state.view().confirmedValue, null);
        assert.equal(state.view().value, 0);
        assert.equal(state.view().busy, false);
        state.dispose();
    }
});

test('true identity replacement resets state and rejects delayed outcomes across A to B to A', async () => {
    const pending = deferred();
    const state = createSteppedInteraction({ interval: 0, send: () => pending.promise });
    state.update(target());
    state.choose(-1);
    state.update(target({ key: 'physical-B' }));
    state.update(target());
    pending.resolve({ success: false, error: 'Old A failure' });
    await flush();
    assert.equal(state.view().failure, '');
    assert.equal(state.view().value, 0);
    state.dispose();
});

test('a connection epoch change rejects delayed success, failure and queued old values', async () => {
    for (const result of [{ success: true, confirmedValue: -1 }, { success: false, error: 'Old epoch failed' }]) {
        const pending = deferred();
        const writes = [];
        const state = createSteppedInteraction({ interval: 0, send: (current, value) => {
            writes.push([current.epoch, value]);
            return pending.promise;
        } });
        state.update(target({ key: '["physical-A",1]', epoch: 1 }));
        state.choose(-1);
        state.choose(-2);
        state.update(target({ key: '["physical-A",2]', epoch: 2 }));
        pending.resolve(result);
        await flush();
        await flush();
        assert.deepEqual(writes, [[1, -1]]);
        assert.equal(state.view().failure, '');
        assert.equal(state.view().confirmedValue, null);
        assert.equal(state.view().value, 0);
        state.dispose();
    }
});

test('a new target choice waits for the old in-flight request and discards its old queued choice', async () => {
    const requests = [];
    const state = createSteppedInteraction({ send: (current, value) => {
        const request = deferred();
        requests.push({ key: current.key, value, request });
        return request.promise;
    } });
    state.update(target());
    state.choose(-1);
    state.choose(-2);
    state.update(target({ key: 'physical-B' }));
    state.choose(-4);
    assert.deepEqual(requests.map(({ key, value }) => [key, value]), [['physical-A', -1]]);
    requests[0].request.resolve({ success: false, error: 'Old target failure' });
    await flush();
    assert.deepEqual(requests.map(({ key, value }) => [key, value]), [['physical-A', -1], ['physical-B', -4]]);
    assert.equal(state.view().failure, '');
    assert.equal(state.view().value, -4);
    requests[1].request.resolve({ success: true, confirmedValue: -4 });
    await flush();
    assert.equal(state.view().confirmedValue, -4);
    state.dispose();
});
