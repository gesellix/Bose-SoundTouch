// The Logs tab's state machine has no DOM or Preact dependency so polling,
// cancellation and filtering can be tested with a fake clock and transport.
export function formatLogLines(entries, filter = '') {
    const query = filter.trim().toLowerCase();
    return entries.filter(entry => !query || entry.message.toLowerCase().includes(query))
        .map(entry => {
            const time = entry.time ? entry.time.replace('T', ' ').replace('Z', '') : '';
            return `${time}  ${entry.message}`;
        }).join('\n');
}

export function createLogsController({
    request,
    maxEntries = 5000,
    intervalMs = 1500,
    schedule = (callback, ms) => setInterval(callback, ms),
    unschedule = timer => clearInterval(timer),
    isHidden = () => globalThis.document?.hidden ?? false,
    now = () => new Date().toLocaleTimeString(),
}) {
    let state = {
        entries: [], nextSince: 0, droppedTotal: 0,
        filter: '', followTail: true, status: 'Idle.',
    };
    let active = false;
    let timer = null;
    let generation = 0;
    let inFlight = null;
    const listeners = new Set();
    const publish = patch => {
        state = { ...state, ...patch };
        for (const listener of listeners) listener(state);
    };
    const poll = async () => {
        if (!active || isHidden() || inFlight) return;
        const version = generation;
        const controller = new AbortController();
        inFlight = controller;
        try {
            const data = await request(state.nextSince, controller.signal);
            if (version !== generation || !active) return;
            const entries = Array.isArray(data.entries)
                ? state.entries.concat(data.entries).slice(-maxEntries)
                : state.entries;
            const nextSince = typeof data.nextSince === 'number' ? data.nextSince : state.nextSince;
            const droppedTotal = state.droppedTotal + (typeof data.dropped === 'number' && data.dropped > 0 ? data.dropped : 0);
            const droppedNote = droppedTotal > 0 ? ` · ${droppedTotal} dropped` : '';
            publish({
                entries, nextSince, droppedTotal,
                status: `${entries.length} buffered${droppedNote} · last update ${now()}`,
            });
        } catch (error) {
            if (version === generation && active && error?.name !== 'AbortError') {
                publish({ status: `Polling failed: ${error?.message || error}` });
            }
        } finally {
            if (inFlight === controller) inFlight = null;
        }
    };
    return {
        snapshot: () => state,
        subscribe(listener) {
            listeners.add(listener);
            listener(state);
            return () => listeners.delete(listener);
        },
        start() {
            if (active) return false;
            active = true;
            generation++;
            publish({ entries: [], nextSince: 0, droppedTotal: 0, status: 'Idle.' });
            void poll();
            timer = schedule(() => { void poll(); }, intervalMs);
            return true;
        },
        stop() {
            if (!active) return;
            active = false;
            generation++;
            if (timer !== null) unschedule(timer);
            timer = null;
            inFlight?.abort();
            inFlight = null;
        },
        setFilter(filter) { publish({ filter }); },
        setFollowTail(followTail) { publish({ followTail }); },
    };
}
