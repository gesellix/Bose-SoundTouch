// TEMPORARY INSTRUMENTATION -- do not merge.
//
// Counts the player's HTTP calls and the speaker events arriving over the
// WebSocket, to measure the source-selection readback cost in PR #670 review
// finding 2. Delete this file and its import in app.js once done.
//
// Off unless localStorage.aftertouchTrace === '1', so it costs nothing until
// switched on. In the browser console:
//
//   localStorage.aftertouchTrace = '1'; location.reload();
//   __trace.mark('click AUX');   // before each thing you want to bracket
//   __trace.report();            // grouped counts since the last mark
//   __trace.reset();
//   delete localStorage.aftertouchTrace; location.reload();

let enabled = false;
try {
    enabled = localStorage.getItem('aftertouchTrace') === '1';
} catch (_) {
    enabled = false;
}

const started = performance.now();
const entries = [];
let markLabel = 'start';
let markAt = started;

function since() {
    return ((performance.now() - started) / 1000).toFixed(3);
}

function record(kind, detail, extra = {}) {
    const entry = { kind, detail, at: performance.now(), mark: markLabel, ...extra };
    entries.push(entry);
    const offset = ((entry.at - markAt) / 1000).toFixed(3);
    console.log(`[trace] t=${since()}s +${offset}s after "${markLabel}"  ${kind}  ${detail}`,
        Object.keys(extra).length ? extra : '');
}

export function installTrace() {
    if (!enabled) return;

    const nativeFetch = globalThis.fetch.bind(globalThis);
    globalThis.fetch = async (input, init) => {
        const url = typeof input === 'string' ? input : input?.url ?? String(input);
        const method = init?.method ?? (typeof input === 'object' ? input?.method : null) ?? 'GET';
        const at = performance.now();
        try {
            const response = await nativeFetch(input, init);
            record('HTTP', `${method} ${url}`, {
                status: response.status,
                ms: Math.round(performance.now() - at),
            });
            return response;
        } catch (error) {
            record('HTTP', `${method} ${url}`, { status: 'ERR', ms: Math.round(performance.now() - at) });
            throw error;
        }
    };

    const NativeWebSocket = globalThis.WebSocket;
    globalThis.WebSocket = function TracedWebSocket(url, protocols) {
        const socket = protocols === undefined
            ? new NativeWebSocket(url) : new NativeWebSocket(url, protocols);
        record('WS', `open ${url}`);
        socket.addEventListener('message', event => {
            let type = 'unparsed';
            let deviceId = '';
            let extra = {};
            try {
                const msg = JSON.parse(event.data);
                type = msg.type ?? 'untyped';
                deviceId = msg.deviceId ?? '';
                // The two fields this investigation cares about.
                const status = msg.data?.status ?? msg.data?.[deviceId]?.status;
                if (status) {
                    extra = {
                        source: status.nowPlaying?.Source,
                        revision: status.revision,
                        nowPlayingRevision: status.nowPlayingRevision,
                        epoch: status.epoch,
                    };
                }
            } catch (_) { /* keep the frame counted even if it is not JSON */ }
            record('WS', `${type}${deviceId ? ' ' + deviceId : ''}`, extra);
        });
        socket.addEventListener('close', () => record('WS', `close ${url}`));
        return socket;
    };
    globalThis.WebSocket.prototype = NativeWebSocket.prototype;

    globalThis.__trace = {
        mark(label) {
            markLabel = label;
            markAt = performance.now();
            console.log(`[trace] ---- mark: ${label} (t=${since()}s) ----`);
        },
        report() {
            const scoped = entries.filter(e => e.mark === markLabel);
            const byDetail = new Map();
            for (const e of scoped) {
                const key = `${e.kind}  ${e.detail}`;
                byDetail.set(key, (byDetail.get(key) ?? 0) + 1);
            }
            console.log(`[trace] since "${markLabel}": ${scoped.length} events`);
            console.table([...byDetail.entries()]
                .sort((a, b) => b[1] - a[1])
                .map(([what, count]) => ({ count, what })));
            return scoped;
        },
        entries: () => entries,
        reset() {
            entries.length = 0;
            markAt = performance.now();
        },
    };

    console.log('[trace] AfterTouch player tracing on. __trace.mark(label) / __trace.report()');
}
