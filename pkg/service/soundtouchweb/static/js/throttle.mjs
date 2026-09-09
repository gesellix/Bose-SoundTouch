// Rate limiting for controls that fire continuously.
//
// This lives in its own framework-free module on purpose. The frontend tests
// run under plain `node --test` with no npm install (see
// .github/workflows/browser-tests.yml), so anything they import must not pull
// in preact — which is why the pure logic in this project sits in .mjs modules
// beside the components rather than inside them.

// throttleTrailing limits how often fn runs, and always runs it once more with
// the final arguments after the last call.
//
// Range inputs fire on every pixel of a drag. Without this, one drag of the
// volume slider is dozens of POSTs to the speaker, and a drag of the balance
// slider is worse still: each write is validated against the range the device
// reports. The trailing call is what makes throttling safe here — the value
// the user let go on is always the value that gets sent, which a plain
// leading-edge throttle would drop.
export function throttleTrailing(fn, ms) {
    let last = 0;
    let timer = null;
    let pending = null;

    return (...args) => {
        pending = args;

        const elapsed = Date.now() - last;
        if (elapsed >= ms) {
            last = Date.now();
            fn(...pending);
            pending = null;
            return;
        }

        if (timer === null) {
            timer = setTimeout(() => {
                timer = null;
                if (pending === null) return;
                last = Date.now();
                fn(...pending);
                pending = null;
            }, ms - elapsed);
        }
    };
}

// SLIDER_WRITE_INTERVAL_MS is the shortest gap between two writes from one
// slider. Long enough to collapse a drag into a handful of requests, short
// enough that dragging still feels live.
export const SLIDER_WRITE_INTERVAL_MS = 150;
