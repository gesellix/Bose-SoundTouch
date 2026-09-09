import assert from 'node:assert/strict';
import test from 'node:test';
import { setTimeout as sleep } from 'node:timers/promises';

import { throttleTrailing } from '../static/js/throttle.mjs';

test('a drag collapses into far fewer writes than input events', async () => {
    const sent = [];
    const write = throttleTrailing((value) => sent.push(value), 50);

    // 40 input events, as a range input fires them during a drag.
    for (let value = 1; value <= 40; value++) {
        write(value);
    }

    await sleep(120);

    assert.ok(sent.length < 5, `sent ${sent.length} writes for 40 input events`);
});

test('the value the user let go on is always sent', async () => {
    const sent = [];
    const write = throttleTrailing((value) => sent.push(value), 50);

    write(1);
    write(2);
    write(7); // the value the pointer was released on

    await sleep(120);

    assert.equal(sent.at(-1), 7, 'the final value must reach the speaker');
});

test('a single change is sent immediately, not delayed', () => {
    const sent = [];
    const write = throttleTrailing((value) => sent.push(value), 50);

    write(3);

    // No await: a lone click on the slider track must not wait out the window.
    assert.deepEqual(sent, [3]);
});

test('writes resume after the window elapses', async () => {
    const sent = [];
    const write = throttleTrailing((value) => sent.push(value), 20);

    write(1);
    await sleep(60);
    write(2);

    assert.deepEqual(sent, [1, 2]);
});
