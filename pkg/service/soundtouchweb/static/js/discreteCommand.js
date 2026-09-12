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

function statusEpoch(status) {
    const epoch = status?.epoch;
    return Number.isSafeInteger(epoch) ? epoch : null;
}

// The revision a command is verified against, tagged with the epoch it was
// counted in: mute is volume state and advances the aggregate revision, every
// other command advances the now-playing one.
function commandRevision(status, command) {
    const revision = commandAction(command)?.startsWith('mute-')
        ? statusRevision(status)
        : nowPlayingRevision(status);
    return revision === null ? null : { epoch: statusEpoch(status), revision };
}

// Revisions are only comparable within one epoch. A device id backed by a new
// DeviceConnection, or a restarted service, restarts its revisions at 0, so a
// reconnect in the middle of a command would otherwise leave every readback
// looking older than the revision the command started at, and the command
// could never confirm. Same rule as app.js acceptsNewerStatus.
function isNewerRevision(start, candidate) {
    if (!start || !candidate) return false;
    if (start.epoch !== null && candidate.epoch !== null && candidate.epoch !== start.epoch) {
        return candidate.epoch > start.epoch;
    }
    return candidate.revision > start.revision;
}

// Track skips cannot be confirmed by reading the speaker back. Confirming on
// "what is playing changed" is wrong in both directions: a live stream rolls
// its track metadata over by itself, so a skip that did nothing looks
// confirmed, while AUX, PRODUCT and radio have no track concept at all, so a
// skip that worked never changes anything and would always end unverified
// after a full readback window of dead transport. A write the speaker
// accepted is the only honest evidence available, so these settle on it.
function isTrackSkip(action) {
    return action === 'next-track' || action === 'previous-track';
}

// What identifies "the same track" where the speaker reports no trackID.
// Stringified so an absent field cannot merge into its neighbour.
export function trackMetadataIdentity(nowPlaying) {
    return JSON.stringify([
        nowPlaying?.Track,
        nowPlaying?.Artist,
        nowPlaying?.Album,
        nowPlaying?.ContentItem?.Location,
    ].map(value => value || ''));
}

const EMPTY_TRACK_IDENTITY = trackMetadataIdentity(null);

// What a skip against this source can be confirmed with, measured on a
// SoundTouch 10 (server version 4) by walking its sources:
//
//   SPOTIFY       trackID, skipEnabled          -> the trackID is the signal
//   STORED_MUSIC  no trackID, skipEnabled       -> track metadata is the signal
//   TUNEIN        no trackID, no skipEnabled    -> nothing to verify
//   RADIO_BROWSER no trackID, no skipEnabled    -> nothing to verify
//   AUX           no trackID, no skipEnabled    -> nothing to verify
//
// trackID is preferred wherever it exists: it is a stable per-track identity
// (`spotify:track:...`), so it survives a stream rewriting its own title and
// still moves on a real skip.
//
// Track metadata is the fallback, and only where the speaker claims the skip
// is supported. That claim is what separates a library, whose title changes
// only when the track really changes, from live radio, which rewrites its
// title while the same stream plays on and claims no skip support at all.
//
// With neither, no readback can ever say anything, so the command settles on a
// write the speaker accepted instead of spending the whole readback window
// with the transport dead to end "unverified".
function nowPlayingPosition(nowPlaying) {
    const position = nowPlaying?.Time?.Position;
    return Number.isSafeInteger(position) && position >= 0 ? position : null;
}

export function trackSkipExpectation(nowPlaying, skipSupported) {
    return {
        previousTrackID: nowPlaying?.TrackID || '',
        previousTrackIdentity: skipSupported ? trackMetadataIdentity(nowPlaying) : '',
        previousPosition: nowPlayingPosition(nowPlaying),
    };
}

function settlesOnWrite(action, expected) {
    return isTrackSkip(action) &&
        !expected?.previousTrackID &&
        (!expected?.previousTrackIdentity ||
            expected.previousTrackIdentity === EMPTY_TRACK_IDENTITY);
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
    if (action === 'play') {
        // Internet radio reports BUFFERING_STATE for seconds at a time before
        // the first audio arrives, sometimes past the last readback deadline.
        // That is the command working, so treat it as confirmed -- but only
        // for a real source, since a speaker in standby is not starting
        // anything.
        return nowPlaying?.PlayStatus === 'PLAY_STATE' ||
            (nowPlaying?.PlayStatus === 'BUFFERING_STATE' &&
                Boolean(nowPlaying.Source) && nowPlaying.Source !== 'STANDBY');
    }
    if (action === 'mute-on') return status?.volume?.MuteEnabled === true;
    if (action === 'mute-off') return status?.volume?.MuteEnabled === false;
    if (action === 'shuffle-on') return nowPlaying?.ShuffleSetting === 'SHUFFLE_ON';
    if (action === 'shuffle-off') return nowPlaying?.ShuffleSetting === 'SHUFFLE_OFF';
    if (action === 'repeat-all') return nowPlaying?.RepeatSetting === 'REPEAT_ALL';
    if (action === 'repeat-one') return nowPlaying?.RepeatSetting === 'REPEAT_ONE';
    if (action === 'repeat-off') return nowPlaying?.RepeatSetting === 'REPEAT_OFF';
    if (isTrackSkip(action)) {
        const expected = command?.expected;
        // Measured on a SoundTouch 10: the first PREV_TRACK restarts the
        // track that is playing, and only a second press steps back. A
        // restart moves no identity at all, so the play position falling back
        // is the only evidence that the press did anything -- without it a
        // working Previous reports "unverified" every time it lands on a
        // track that is already under way.
        if (action === 'previous-track' && expected?.previousPosition > 0) {
            const position = nowPlayingPosition(nowPlaying);
            if (position !== null && position < expected.previousPosition) return true;
        }
        if (expected?.previousTrackID) {
            const trackID = nowPlaying?.TrackID;
            return Boolean(trackID) && trackID !== expected.previousTrackID;
        }
        if (expected?.previousTrackIdentity) {
            const identity = trackMetadataIdentity(nowPlaying);
            return identity !== EMPTY_TRACK_IDENTITY &&
                identity !== expected.previousTrackIdentity;
        }
        return false;
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
        if (!command || !isNewerRevision(command.startRevision, revision) ||
            command.outcome === 'failed' || command.outcome === 'unverified') return;

        const matches = matchesCommand(status, command);
        if (command.outcome === 'final-confirmed') {
            if (isNewerRevision(command.confirmedRevision, revision) && !matches) {
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
        // A command that settles on its write schedules no readbacks at all.
        const delays = settlesOnWrite(action, expected) ? [] : readbackDelays;

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

        // Settles a command the speaker accepted but nothing will read back.
        function confirmOnWrite() {
            if (commandRef.current.active !== active) return;
            clearReadbacks();
            commandRef.current.active = null;
            setCommand(previous => previous?.generation === generation
                ? {
                    ...active.request,
                    generation,
                    outcome: 'final-confirmed',
                    startRevision,
                    confirmedRevision: commandRevision(statusRef.current, active.request),
                }
                : previous);
        }

        delays.forEach((delay, index) => {
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
                        !isNewerRevision(currentRevision, readbackRevision)
                        ? currentStatus
                        : readbackStatus;
                    const canonicalRevision = commandRevision(canonicalStatus, currentRequest);
                    const revisionIsNewer = isNewerRevision(startRevision, canonicalRevision);
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
                        const isFinalReadback = index === delays.length - 1;
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
                    } else if (index === delays.length - 1) {
                        unverified();
                    }
                } catch (error) {
                    if (commandRef.current.active === active && active.latestReadback === index &&
                        index === delays.length - 1) {
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
            // A response-level failure carries no status code, so it is no
            // proof the speaker never acted: record it and keep verifying.
            // Checked writes reject instead, and are classified below.
            if (response?.success === false) {
                active.writeError = new Error(response.error || 'Command rejected');
                if (settlesOnWrite(action, expected)) unverified();
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
            if (settlesOnWrite(action, expected)) confirmOnWrite();
        }).catch(error => {
            if (commandRef.current.active !== active) return;
            // A definitive refusal (4xx) means the speaker never saw the
            // command, so there is nothing for the readbacks to confirm and
            // reporting it now beats waiting out the readback window. Anything
            // else stays pending: a 5xx or a transport error does not tell us
            // whether the speaker acted -- a request that timed out after the
            // speaker already switched looks exactly like one it never
            // received -- so we keep verifying and carry the reason into
            // whatever outcome the readbacks reach.
            if (error?.definitive) {
                fail(error);
                return;
            }
            active.writeError = error;
            // Nothing will read this one back, so the ambiguous failure is as
            // far as it gets.
            if (settlesOnWrite(action, expected)) unverified(error);
        });
        return true;
    }

    const busy = command?.outcome === 'pending' ||
        command?.outcome === 'provisional-confirmed';
    return { command, busy, statusText: commandText(command), run };
}
