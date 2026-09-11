import { useState, useEffect, useRef } from 'preact/hooks';
import { api } from './api.js';

export const DISCRETE_COMMAND_READBACK_DELAYS_MS = [2000, 5000, 10000];

function statusRevision(status) {
    const revision = status?.revision;
    return Number.isSafeInteger(revision) && revision >= 0 ? revision : null;
}

function nowPlayingRevision(status) {
    const revision = status?.nowPlayingRevision;
    return Number.isSafeInteger(revision) && revision >= 0 ? revision : null;
}

function commandAction(command) {
    return typeof command === 'string' ? command : command?.action;
}

function commandRevision(status, command) {
    return commandAction(command)?.startsWith('mute-')
        ? statusRevision(status)
        : nowPlayingRevision(status);
}

export function playbackIdentity(nowPlaying) {
    if (!nowPlaying) return '';
    const item = nowPlaying.ContentItem || {};
    return [
        nowPlaying.Source,
        nowPlaying.SourceAccount,
        item.Source,
        item.SourceAccount,
        item.Location,
        nowPlaying.TrackID,
        nowPlaying.Track,
        nowPlaying.StationName,
    ].map(value => value || '').join('\u0000');
}

export function contentExpectation(item) {
    const source = item?.Source || '';
    const sourceAccount = item?.SourceAccount && item.SourceAccount !== source
        ? item.SourceAccount
        : '';
    return {
        source,
        sourceAccount,
        location: item?.Location || '',
        itemName: item?.ItemName || '',
    };
}

function matchesContentExpectation(nowPlaying, expected) {
    if (!nowPlaying || !expected) return false;
    const item = nowPlaying.ContentItem || {};
    const source = item.Source || nowPlaying.Source || '';
    const account = item.SourceAccount || nowPlaying.SourceAccount || '';
    if (expected.source && source !== expected.source) return false;
    if (expected.sourceAccount && account !== expected.sourceAccount) return false;
    if (expected.location) {
        return item.Location === expected.location ||
            nowPlaying.StationLocation === expected.location;
    }
    if (expected.itemName) return [
        item.ItemName,
        nowPlaying.Track,
        nowPlaying.StationName,
    ].includes(expected.itemName);
    return Boolean(expected.source);
}

export function matchesCommand(status, command) {
    if (command?.expectationReady === false) return false;
    const action = commandAction(command);
    const nowPlaying = status?.nowPlaying;
    if (action === 'power-off') return nowPlaying?.Source === 'STANDBY';
    if (action === 'power-on') return Boolean(nowPlaying?.Source) && nowPlaying.Source !== 'STANDBY';
    if (action === 'pause') {
        return nowPlaying?.PlayStatus === 'PAUSE_STATE' ||
            (nowPlaying?.PlayStatus === 'STOP_STATE' &&
                Boolean(nowPlaying.Source) && nowPlaying.Source !== 'STANDBY');
    }
    if (action === 'play') return nowPlaying?.PlayStatus === 'PLAY_STATE';
    if (action === 'mute-on') return status?.volume?.MuteEnabled === true;
    if (action === 'mute-off') return status?.volume?.MuteEnabled === false;
    if (action === 'shuffle-on') return nowPlaying?.ShuffleSetting === 'SHUFFLE_ON';
    if (action === 'shuffle-off') return nowPlaying?.ShuffleSetting === 'SHUFFLE_OFF';
    if (action === 'repeat-all') return nowPlaying?.RepeatSetting === 'REPEAT_ALL';
    if (action === 'repeat-one') return nowPlaying?.RepeatSetting === 'REPEAT_ONE';
    if (action === 'repeat-off') return nowPlaying?.RepeatSetting === 'REPEAT_OFF';
    if (action === 'next-track' || action === 'previous-track') {
        const identity = playbackIdentity(nowPlaying);
        return Boolean(identity) && identity !== command?.expected?.previousIdentity;
    }
    if (['preset', 'recent', 'tunein', 'radiobrowser', 'url', 'library'].includes(action)) {
        return matchesContentExpectation(nowPlaying, command?.expected);
    }
    return false;
}

function commandFailed(status, command) {
    const action = commandAction(command);
    if (![
        'play', 'pause', 'next-track', 'previous-track', 'preset', 'recent',
        'tunein', 'radiobrowser', 'url', 'library',
    ].includes(action)) return false;
    const nowPlaying = status?.nowPlaying;
    return nowPlaying?.Source === 'INVALID_SOURCE' ||
        nowPlaying?.Source?.endsWith('_ERROR') ||
        nowPlaying?.PlayStatus === 'INVALID_PLAY_STATE';
}

export function commandText(command) {
    if (!command) return '';
    const labels = {
        'power-off': ['Turning device off', 'Device powered off, confirming', 'Device powered off', 'Power command unverified', 'Power command failed'],
        'power-on': ['Waking device', 'Device awake, confirming', 'Device awake', 'Power command unverified', 'Power command failed'],
        pause: ['Pausing playback', 'Playback paused, confirming', 'Playback paused', 'Pause command unverified', 'Pause command failed'],
        play: ['Starting playback', 'Playback started, confirming', 'Playback started', 'Play command unverified', 'Play command failed'],
        'mute-on': ['Muting audio', 'Audio muted, confirming', 'Audio muted', 'Mute command unverified', 'Mute command failed'],
        'mute-off': ['Unmuting audio', 'Audio unmuted, confirming', 'Audio unmuted', 'Unmute command unverified', 'Unmute command failed'],
        'shuffle-on': ['Enabling shuffle', 'Shuffle enabled, confirming', 'Shuffle enabled', 'Shuffle command unverified', 'Shuffle command failed'],
        'shuffle-off': ['Disabling shuffle', 'Shuffle disabled, confirming', 'Shuffle disabled', 'Shuffle command unverified', 'Shuffle command failed'],
        'repeat-all': ['Enabling repeat all', 'Repeat all enabled, confirming', 'Repeat all enabled', 'Repeat command unverified', 'Repeat command failed'],
        'repeat-one': ['Enabling repeat one', 'Repeat one enabled, confirming', 'Repeat one enabled', 'Repeat command unverified', 'Repeat command failed'],
        'repeat-off': ['Disabling repeat', 'Repeat disabled, confirming', 'Repeat disabled', 'Repeat command unverified', 'Repeat command failed'],
        'next-track': ['Skipping to next track', 'Next track started, confirming', 'Next track started', 'Next-track command unverified', 'Next-track command failed'],
        'previous-track': ['Returning to previous track', 'Previous track started, confirming', 'Previous track started', 'Previous-track command unverified', 'Previous-track command failed'],
        preset: ['Starting preset', 'Preset started, confirming', 'Preset started', 'Preset playback unverified', 'Preset playback failed'],
        recent: ['Starting recent item', 'Recent item started, confirming', 'Recent item started', 'Recent-item playback unverified', 'Recent-item playback failed'],
        tunein: ['Starting TuneIn item', 'TuneIn item started, confirming', 'TuneIn item started', 'TuneIn playback unverified', 'TuneIn playback failed'],
        radiobrowser: ['Starting RadioBrowser station', 'RadioBrowser station started, confirming', 'RadioBrowser station started', 'RadioBrowser playback unverified', 'RadioBrowser playback failed'],
        url: ['Starting stream URL', 'Stream URL started, confirming', 'Stream URL started', 'Stream URL playback unverified', 'Stream URL playback failed'],
        library: ['Starting library item', 'Library item started, confirming', 'Library item started', 'Library playback unverified', 'Library playback failed'],
    };
    const outcomes = ['pending', 'provisional-confirmed', 'final-confirmed', 'unverified', 'failed'];
    const index = outcomes.indexOf(command.outcome);
    return index >= 0 ? labels[command.action]?.[index] || '' : '';
}

export function useDiscreteCommand({
    deviceId,
    status,
    onStatusReadback,
    readbackDelays = DISCRETE_COMMAND_READBACK_DELAYS_MS,
    targetIdentity = null,
    currentTargetIdentity = null,
}) {
    const [command, setCommand] = useState(null);
    const commandRef = useRef({ generation: 0, active: null, timers: [] });
    const statusRef = useRef(status);
    const currentTargetIdentityRef = useRef(currentTargetIdentity);
    statusRef.current = status;
    currentTargetIdentityRef.current = currentTargetIdentity;

    function clearReadbacks() {
        commandRef.current.timers.forEach(clearTimeout);
        commandRef.current.timers = [];
    }

    useEffect(() => {
        commandRef.current.generation += 1;
        commandRef.current.active = null;
        clearReadbacks();
        setCommand(null);
        return () => {
            commandRef.current.generation += 1;
            commandRef.current.active = null;
            clearReadbacks();
        };
    }, [deviceId]);

    useEffect(() => {
        if (!command?.targetIdentity || currentTargetIdentity === command.targetIdentity ||
            command.outcome === 'failed' || command.outcome === 'unverified') return;

        if (command.outcome === 'final-confirmed') {
            setCommand(previous => previous?.generation === command.generation
                ? null : previous);
            return;
        }

        clearReadbacks();
        commandRef.current.active = null;
        setCommand(previous => previous?.generation === command.generation
            ? { ...previous, outcome: 'failed', error: 'Playback target changed' }
            : previous);
    }, [command, currentTargetIdentity]);

    useEffect(() => {
        const revision = commandRevision(status, command);
        if (!command || revision === null || command.startRevision === null ||
            revision <= command.startRevision || command.outcome === 'failed' ||
            command.outcome === 'unverified') return;

        const matches = matchesCommand(status, command);
        if (command.outcome === 'final-confirmed') {
            if (revision > command.confirmedRevision && !matches) {
                setCommand(previous => previous?.generation === command.generation
                    ? null : previous);
            }
            return;
        }
        if (commandFailed(status, command)) {
            clearReadbacks();
            commandRef.current.active = null;
            setCommand(previous => previous?.generation === command.generation
                ? { ...previous, outcome: 'failed' }
                : previous);
        } else if (matches && command.outcome === 'pending') {
            setCommand(previous => previous?.generation === command.generation
                ? { ...previous, outcome: 'provisional-confirmed' }
                : previous);
        }
    }, [command, status]);

    function run(action, invoke, expected = null, options = {}) {
        if (commandRef.current.active) return false;

        clearReadbacks();
        const generation = commandRef.current.generation + 1;
        const request = {
            action,
            expected,
            expectationReady: !options.expectedFromResponse,
            targetIdentity,
        };
        const startRevision = commandRevision(status, request);
        const active = {
            generation,
            latestReadback: -1,
            writeError: null,
            request,
            startRevision,
        };
        commandRef.current.generation = generation;
        commandRef.current.active = active;
        setCommand({ ...request, generation, outcome: 'pending', startRevision });
        const startedAt = Date.now();

        function fail(error) {
            if (commandRef.current.active !== active) return;
            clearReadbacks();
            commandRef.current.active = null;
            setCommand({
                ...active.request,
                generation,
                outcome: 'failed',
                startRevision,
                error: error?.message || String(error || 'Command rejected'),
            });
        }

        // Called when the readback window closes without this round matching.
        // "Unverified" is only honest for a command nothing ever confirmed: a
        // nowPlayingUpdated event may already have confirmed it at t=1s, and a
        // failed readback at t=10s must not retract that. Such a command is
        // settled as confirmed instead, since no further readback will run.
        function unverified(error) {
            if (commandRef.current.active !== active) return;
            commandRef.current.active = null;
            setCommand(previous => {
                if (previous?.generation !== generation) return previous;
                if (previous.outcome === 'provisional-confirmed' ||
                    previous.outcome === 'final-confirmed') {
                    return { ...previous, outcome: 'final-confirmed' };
                }
                if (previous.outcome === 'failed') return previous;

                return {
                    ...active.request,
                    generation,
                    outcome: 'unverified',
                    startRevision,
                    error: active.writeError?.message || error?.message,
                };
            });
        }

        readbackDelays.forEach((delay, index) => {
            const timer = setTimeout(async () => {
                if (commandRef.current.active !== active) return;
                active.latestReadback = index;
                try {
                    // Mute is volume state, which the lightweight now-playing
                    // endpoint does not refresh from the speaker. Other
                    // discrete commands only need the much cheaper media
                    // readback.
                    const response = commandAction(active.request)?.startsWith('mute-')
                        ? await api.device(deviceId)
                        : await api.deviceNowPlaying(deviceId);
                    if (commandRef.current.active !== active || active.latestReadback !== index) return;
                    if (response?.success === false || !response?.data?.status) {
                        throw new Error(response?.error || 'Device status unavailable');
                    }

                    const currentRequest = active.request;
                    if (currentRequest.targetIdentity && (
                        currentTargetIdentityRef.current !== currentRequest.targetIdentity ||
                        response.data.info?.device_id !== currentRequest.targetIdentity
                    )) {
                        fail(new Error('Playback target changed'));
                        return;
                    }

                    const readbackStatus = response.data.status;
                    const readbackRevision = commandRevision(readbackStatus, currentRequest);
                    const currentStatus = statusRef.current;
                    const currentRevision = commandRevision(currentStatus, currentRequest);
                    const canonicalStatus = currentRevision !== null &&
                        (readbackRevision === null || currentRevision >= readbackRevision)
                        ? currentStatus
                        : readbackStatus;
                    const canonicalRevision = commandRevision(canonicalStatus, currentRequest);
                    const revisionIsNewer = startRevision !== null && canonicalRevision !== null &&
                        canonicalRevision > startRevision;
                    onStatusReadback?.(deviceId, readbackStatus, response.data.info);

                    if (revisionIsNewer && commandFailed(canonicalStatus, currentRequest)) {
                        fail(new Error('Device rejected the selected source'));
                    } else if (revisionIsNewer && matchesCommand(canonicalStatus, currentRequest)) {
                        // A match is not always the end of the story: the
                        // speaker can still transition into an error source a
                        // few seconds later, so something has to keep watching.
                        //
                        // The event stream is the better watcher when it is
                        // live: it reports that transition as it happens, and
                        // the status effect above already turns it into a
                        // failure. Polling on top of that only re-asks a
                        // question we are already subscribed to the answer of,
                        // and keeps every command button disabled until the
                        // last deadline passes. So keep the remaining
                        // readbacks only as a fallback for a device whose
                        // events we are not receiving.
                        const isFinalReadback = index === readbackDelays.length - 1;
                        const eventStreamWatching = readbackStatus?.webSocketConnected === true;
                        const settled = isFinalReadback || eventStreamWatching;
                        setCommand({
                            ...currentRequest,
                            generation,
                            outcome: settled ? 'final-confirmed' : 'provisional-confirmed',
                            startRevision,
                            confirmedRevision: canonicalRevision,
                        });
                        if (settled) {
                            clearReadbacks();
                            commandRef.current.active = null;
                        }
                    } else if (index === readbackDelays.length - 1) {
                        unverified();
                    }
                } catch (error) {
                    if (commandRef.current.active === active && active.latestReadback === index &&
                        index === readbackDelays.length - 1) {
                        unverified(error);
                    }
                }
            }, Math.max(0, delay - (Date.now() - startedAt)));
            commandRef.current.timers.push(timer);
        });

        if (request.targetIdentity &&
            currentTargetIdentityRef.current !== request.targetIdentity) {
            fail(new Error('Playback target changed'));
            return true;
        }

        let write;
        try {
            write = invoke();
        } catch (error) {
            fail(error);
            return true;
        }
        Promise.resolve(write).then(response => {
            if (commandRef.current.active !== active) return;
            if (response?.success === false) {
                fail(new Error(response.error || 'Command rejected'));
                return;
            }
            if (options.expectedFromResponse) {
                let refinedExpected;
                try {
                    refinedExpected = options.expectedFromResponse(response);
                } catch (error) {
                    fail(error);
                    return;
                }
                if (!refinedExpected) {
                    fail(new Error('Command response did not identify the selected content'));
                    return;
                }
                active.request = {
                    ...active.request,
                    expected: refinedExpected,
                    expectationReady: true,
                };
                setCommand(previous => previous?.generation === generation
                    ? { ...previous, expected: refinedExpected, expectationReady: true }
                    : previous);
            }
        }).catch(error => {
            if (commandRef.current.active === active) active.writeError = error;
        });
        return true;
    }

    const busy = command?.outcome === 'pending' ||
        command?.outcome === 'provisional-confirmed';
    return { command, busy, statusText: commandText(command), run };
}
