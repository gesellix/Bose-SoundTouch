import { createLatestWinsScheduler, shouldSurfaceLatestFinal } from './latestWinsScheduler.mjs';

export function nearestStep(clientX, left, width, min, max) {
    if (![clientX, left, width, min, max].every(Number.isFinite) || width <= 0 ||
        !Number.isInteger(min) || !Number.isInteger(max) || min > max) return null;
    const fraction = Math.max(0, Math.min(1, (clientX - left) / width));
    return Math.round(min + fraction * (max - min));
}

// Discrete clicks are complete interactions. The same scheduler used by volume
// serializes writes and retains only the newest pending value; focus/blur have
// no mutation handler. Ownership is advanced synchronously on render so an
// old A response cannot settle into a later A after visiting B.
export function createSteppedInteraction({ send, onChange = () => {}, ...timing }) {
    let target = null;
    let owner = 0;
    let eligibility = 0;
    let interaction = 0;
    let tap = null;
    let local = null;
    let failure = '';
    let failureFloor = null;
    let unverifiedValue = null;
    let busy = false;
    let confirmedValue = null;
    let lastCompletedWrite = null;
    const scheduler = createLatestWinsScheduler({
        interval: 0,
        ...timing,
        send(value, request) {
            if (request.owner !== owner || request.eligibility !== eligibility) return { skipped: true };
            if (target?.disabled) return { success: false, error: 'Sound setting is currently unavailable. Refresh settings before retrying.' };
            if (!Number.isInteger(value) || value < target.min || value > target.max) {
                return { success: false, error: 'The reported sound range changed. Refresh settings before retrying.' };
            }
            request.dispatched = true;
            return send(target, value);
        },
        onResult(response, request) {
            if (request.owner !== owner) return;
            if (request.dispatched) lastCompletedWrite = { response, request };
            if (!shouldSurfaceLatestFinal(request)) return;
            if (response?.skipped) {
                // A cancelled pending choice was never written. Its completion
                // must retain the last sent outcome rather than replace it.
                if (lastCompletedWrite?.request.owner === owner) {
                    if ('error' in lastCompletedWrite) surfaceError(lastCompletedWrite.error);
                    else surfaceResult(lastCompletedWrite.response, lastCompletedWrite.request);
                } else {
                    local = target.value;
                    confirmedValue = null;
                    onChange();
                }
                return;
            }
            surfaceResult(response, request);
        },
        onError(error, request) {
            if (request.owner !== owner) return;
            if (request.dispatched) lastCompletedWrite = { error, request };
            if (shouldSurfaceLatestFinal(request)) surfaceError(error);
        },
        onStateChange(state) {
            busy = state.active;
            onChange();
        },
    });

    function surfaceResult(response, request) {
        if (response?.outcome === 'unverified' && !target.disabled && target.observationConfirmed !== false && request.projection !== target.projection &&
            target.value === request.value && target.value >= target.min && target.value <= target.max) {
            failure = '';
            failureFloor = null;
            unverifiedValue = null;
            confirmedValue = target.value;
            local = target.value;
            onChange();
            return;
        }
        failure = response?.success ? '' : response?.error || 'Sound setting update failed.';
        failureFloor = failure ? target.projection : null;
        unverifiedValue = failure && response?.outcome === 'unverified' ? request.value : null;
        if (request.projection === target.projection && response?.success && Number.isInteger(response.confirmedValue)) {
            confirmedValue = response.confirmedValue;
            local = confirmedValue;
        } else {
            confirmedValue = null;
            local = target.value;
        }
        onChange();
    }

    function surfaceError(error) {
        failure = error?.message || 'Sound setting update failed.';
        failureFloor = target.projection;
        unverifiedValue = null;
        local = target.value;
        confirmedValue = null;
        onChange();
    }

    function update(next) {
        if (target && next.disabled !== target.disabled) {
            eligibility += 1;
            tap = null;
        }
        if (!target || next.key !== target.key) {
            owner += 1;
            lastCompletedWrite = null;
            tap = null;
            local = next.value;
            failure = '';
            failureFloor = null;
            unverifiedValue = null;
            confirmedValue = null;
        } else if (next.projection !== target.projection) {
            confirmedValue = null;
            if (!scheduler.isActive()) local = next.value;
            if (failure && !next.disabled && next.observationConfirmed !== false && next.projection !== failureFloor &&
                Number.isInteger(next.value) && next.value >= next.min && next.value <= next.max &&
                (unverifiedValue === null || next.value === unverifiedValue)) {
                if (unverifiedValue !== null) confirmedValue = next.value;
                failure = '';
                failureFloor = null;
                unverifiedValue = null;
            }
        }
        target = next;
    }

    function choose(value) {
        if (!target || target.disabled || !Number.isInteger(value) ||
            value < target.min || value > target.max || value === local) return;
        interaction += 1;
        local = value;
        failure = '';
        failureFloor = null;
        unverifiedValue = null;
        scheduler.queue(value, { force: true, owner, eligibility, projection: target.projection, interactionGeneration: interaction });
        onChange();
    }

    function step(delta) {
        if (!target || !Number.isInteger(delta)) return;
        choose(Math.max(target.min, Math.min(target.max, local + delta)));
    }

    function keyDown(key) {
        if (!target || target.disabled) return false;
        if (key === 'ArrowLeft' || key === 'ArrowDown') step(-1);
        else if (key === 'ArrowRight' || key === 'ArrowUp') step(1);
        else if (key === 'Home') choose(target.min);
        else if (key === 'End') choose(target.max);
        else return false;
        return true;
    }

    function beginTap(event) {
        tap = null;
        if (!target || target.disabled || event.button !== 0 || event.isPrimary === false) return;
        tap = { id: event.pointerId, x: event.clientX, y: event.clientY, moved: false };
    }

    function moveTap(event) {
        if (!tap || event.pointerId !== tap.id) return;
        // A scroll or drag cannot become a write even if it returns to its start.
        if (Math.hypot(event.clientX - tap.x, event.clientY - tap.y) > 8) tap.moved = true;
    }

    function endTap(event, rect) {
        if (!tap || event.pointerId !== tap.id) return;
        moveTap(event);
        const completed = tap;
        tap = null;
        if (completed.moved || event.clientX < rect.left || event.clientX > rect.left + rect.width ||
            event.clientY < rect.top || event.clientY > rect.top + rect.height) return;
        choose(nearestStep(completed.x, rect.left, rect.width, target.min, target.max));
    }

    return {
        update, choose, step, keyDown, beginTap, moveTap, endTap,
        cancelTap: () => { tap = null; },
        view: () => ({ value: local, failure, busy, confirmedValue }),
        dispose() { tap = null; lastCompletedWrite = null; scheduler.dispose(); },
    };
}
