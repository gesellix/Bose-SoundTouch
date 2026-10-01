export function createLatestWinsScheduler({
    send,
    interval = 200,
    now = () => Date.now(),
    setTimer = (callback, delay) => setTimeout(callback, delay),
    clearTimer = timer => clearTimeout(timer),
    onResult = () => {},
    onError = () => {},
    onStateChange = () => {},
}) {
    let sequence = 0;
    let latestValue;
    let latestInteraction;
    let pending = null;
    let inFlight = null;
    let timer = null;
    let lastStartedAt = -Infinity;
    let lastSettled = null;
    let disposed = false;

    function state() {
        return {
            active: pending !== null || inFlight !== null || timer !== null,
            inFlight: inFlight !== null,
            pending: pending !== null,
            latestSequence: sequence,
            latestValue,
        };
    }

    function emitState() {
        if (!disposed) onStateChange(state());
    }

    function settle(request, kind, value) {
        if (disposed || inFlight !== request) return;

        inFlight = null;
        const metadata = {
            ...request,
            isLatest: request.sequence === sequence && pending === null,
        };
        lastSettled = { kind, value, request: { ...request } };

        if (kind === 'result') onResult(value, metadata);
        else onError(value, metadata);

        pump();
        emitState();
    }

    function start() {
        const request = pending;
        pending = null;
        inFlight = request;
        lastStartedAt = now();
        emitState();

        let result;
        try {
            result = send(request.value, request);
        } catch (error) {
            settle(request, 'error', error);
            return;
        }

        Promise.resolve(result).then(
            response => settle(request, 'result', response),
            error => settle(request, 'error', error),
        );
    }

    function pump() {
        if (disposed || inFlight !== null || pending === null) return;

        const delay = Math.max(0, lastStartedAt + interval - now());
        if (delay === 0) {
            start();
            return;
        }

        if (timer === null) {
            timer = setTimer(() => {
                timer = null;
                pump();
                emitState();
            }, delay);
        }
    }

    function queue(value, { force = false, ...requestMetadata } = {}) {
        if (disposed) return sequence;

        const sameInteraction = requestMetadata.interactionGeneration !== undefined &&
            requestMetadata.interactionGeneration === latestInteraction;

        if (force && value === latestValue && sameInteraction) {
            if (pending?.value === value) {
                pending.final = true;
                emitState();
                return pending.sequence;
            }

            if (inFlight?.value === value && pending === null) {
                inFlight.final = true;
                emitState();
                return inFlight.sequence;
            }

            if (inFlight === null && pending === null &&
                lastSettled?.request.sequence === sequence &&
                lastSettled.request.value === value) {
                const request = {
                    ...lastSettled.request,
                    final: true,
                };
                const metadata = { ...request, isLatest: true };

                if (lastSettled.kind === 'result') onResult(lastSettled.value, metadata);
                else onError(lastSettled.value, metadata);
                emitState();
                return sequence;
            }
        }

        if (!force && value === latestValue && sameInteraction) return sequence;

        sequence += 1;
        latestValue = value;
        latestInteraction = requestMetadata.interactionGeneration;
        pending = { ...requestMetadata, sequence, value, final: force };
        pump();
        emitState();

        return sequence;
    }

    function dispose() {
        disposed = true;
        pending = null;
        lastSettled = null;
        if (timer !== null) clearTimer(timer);
        timer = null;
    }

    return {
        queue,
        dispose,
        isActive: () => state().active,
        getState: state,
    };
}

export function shouldSurfaceLatestFinal(metadata) {
    return Boolean(metadata?.isLatest && metadata?.final);
}
