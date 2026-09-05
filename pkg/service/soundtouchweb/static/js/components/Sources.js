import { h } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

const SOURCE_ICONS = {
    TUNEIN: '📻', SPOTIFY: '🎵', AMAZON: '🛒', PANDORA: '🎶',
    BLUETOOTH: '📶', AUX: '🔌', OPTICAL: '💡', HDMI: '📺',
    IHEARTRADIO: '❤️', DEEZER: '🎼', LOCAL_INTERNET_RADIO: '📡',
    AIRPLAY: '📡', PRODUCT: '🔊',
};

const SOURCE_READBACK_DELAYS_MS = [2000, 5000, 10000];

// Sources the speaker advertises as READY but which are NOT selectable inputs.
// They are providers: playing one needs a station ContentItem carrying a
// Location (see stations.ResolveContentItem, which sets type="stationurl").
//
// A bare /select with just source and account parks the speaker on a stub
// now-playing -- source="RADIO_BROWSER", empty type and location, itemName
// echoing the source name, no playStatus -- while the previous audio keeps
// playing. The speaker then reports that stub indefinitely, so the player
// shows a source the speaker is not actually playing.
//
// Verified against real hardware for RADIO_BROWSER and LOCAL_INTERNET_RADIO:
// both produce the byte-identical stub. TUNEIN is listed because
// ResolveContentItem treats it identically to RADIO_BROWSER (both need a
// Location). ALEXA is also advertised READY but is NOT listed: whether a bare
// select resumes anything for it is unverified, so it keeps today's behaviour.
//
// `page` is the browser to fall back to when there is nothing to resume: the
// page in this app that produces content for that source. LOCAL_INTERNET_RADIO
// maps to Play URL because that is what HandlePlayURL emits, a ContentItem
// with Source "LOCAL_INTERNET_RADIO".
const PROVIDER_SOURCES = {
    RADIO_BROWSER: { page: 'radiobrowser' },
    TUNEIN: { page: 'tunein' },
    LOCAL_INTERNET_RADIO: { page: 'playurl' },
};

function isErrorSource(source) {
    return source === 'INVALID_SOURCE' || source?.endsWith('_ERROR');
}

function sourceAccountIdentity(source, account) {
    return account && account !== source ? account : '';
}

function sourceAccountsMatch(source, left, right) {
    return sourceAccountIdentity(source, left) === sourceAccountIdentity(source, right);
}

export function Sources({
    deviceId,
    status,
    onStatusReadback,
    onNavigate,
    readbackDelays = SOURCE_READBACK_DELAYS_MS,
}) {
    const [command, setCommand] = useState(null);
    const commandRef = useRef({ generation: 0, active: null, timers: [] });
    const mountedRef = useRef(false);
    const items = status?.sources?.SourceItem ?? [];
    const currentSource = status?.nowPlaying?.Source;
    const currentAccount = status?.nowPlaying?.SourceAccount;
    const nowPlayingRevision = Number.isSafeInteger(status?.nowPlayingRevision)
        ? status.nowPlayingRevision : null;
    const sourcesStale = status?.sourcesStale === true;

    function clearReadbacks() {
        commandRef.current.timers.forEach(clearTimeout);
        commandRef.current.timers = [];
    }

    useEffect(() => {
        if (!mountedRef.current) {
            mountedRef.current = true;
            return;
        }

        commandRef.current.generation += 1;
        commandRef.current.active = null;
        clearReadbacks();
        setCommand(null);
    }, [deviceId]);

    useEffect(() => {
        return () => {
            commandRef.current.generation += 1;
            commandRef.current.active = null;
            clearReadbacks();
        };
    }, []);

    useEffect(() => {
        if (!command || nowPlayingRevision === null || command.startNowPlayingRevision === null ||
            nowPlayingRevision <= command.startNowPlayingRevision ||
            command.outcome === 'failed' || command.outcome === 'unverified') return;

        const matches = currentSource === command.source &&
            sourceAccountsMatch(command.source, currentAccount, command.account);
        if (command.outcome === 'final-confirmed') {
            if (nowPlayingRevision > command.confirmedRevision && !matches) {
                setCommand(previous => previous?.generation === command.generation
                    ? null : previous);
            }
            return;
        }
        if (isErrorSource(currentSource)) {
            clearReadbacks();
            commandRef.current.active = null;
            setCommand(previous => previous?.generation === command.generation
                ? { ...previous, outcome: 'failed', error: currentSource }
                : previous);
        } else if (matches && command.outcome === 'pending') {
            setCommand(previous => previous?.generation === command.generation
                ? {
                    ...previous,
                    outcome: 'provisional-confirmed',
                    confirmedRevision: nowPlayingRevision,
                }
                : previous);
        }
    }, [command, currentSource, currentAccount, nowPlayingRevision]);

    const ready = items.filter(s => s.Status === 'READY');
    // Nothing to show and nothing to say: a device that has not been polled
    // yet has no inventory to call out of date. Only render once there is
    // either a list to offer or a list we are refusing to act on.
    if (ready.length === 0 && !sourcesStale) return null;

    const availabilityMessage = sourcesStale
        ? 'Source list out of date'
        : '';
    const availabilityId = sourcesStale ? 'source-stale-status' : null;

    // Finds the most recent playable item for a source: the newest Recents
    // entry for it that carries a Location. Recents entries hold the real
    // ContentItem the speaker was given, which is exactly what a provider
    // source needs and what a bare select cannot supply.
    async function mostRecentPlayableFor(src) {
        const response = await api.recents(deviceId);
        const account = src.SourceAccount ?? '';
        const match = (response?.data?.Items ?? []).find(item => {
            const ci = item?.ContentItem;
            return ci?.Source === src.Source && ci?.Location &&
                sourceAccountsMatch(src.Source, ci.SourceAccount ?? '', account);
        });

        return match?.ContentItem ?? null;
    }

    async function select(src) {
        const provider = PROVIDER_SOURCES[src.Source];
        if (!provider) {
            await runSourceCommand(src,
                () => api.selectSource(deviceId, src.Source, src.SourceAccount ?? ''));

            return;
        }

        let item = null;
        try {
            item = await mostRecentPlayableFor(src);
        } catch (_) {
            item = null;
        }

        // Nothing to resume, and a bare select would strand the speaker on a
        // stub: send the user to the browser to pick something instead.
        if (!item) {
            onNavigate?.(provider.page);

            return;
        }

        await runSourceCommand(src, () => api.playChecked(deviceId, {
            source: item.Source,
            type: item.Type,
            location: item.Location,
            sourceAccount: item.SourceAccount,
            itemName: item.ItemName,
            containerArt: item.ContainerArt,
            isPresetable: item.IsPresetable,
        }));
    }

    async function runSourceCommand(src, write) {
        clearReadbacks();
        const generation = commandRef.current.generation + 1;
        const target = { source: src.Source, account: src.SourceAccount ?? '' };
        commandRef.current.generation = generation;
        const active = { generation, latestReadback: -1, writeError: null };
        commandRef.current.active = active;
        setCommand({
            ...target,
            generation,
            outcome: 'pending',
            startNowPlayingRevision: nowPlayingRevision,
        });
        const startedAt = Date.now();

        // Called when the readback window closes without this round matching.
        // "Unverified" is only honest for a command nothing ever confirmed: a
        // nowPlayingUpdated event may already have confirmed it at t=1s, and a
        // failed readback at t=10s must not retract that. Such a command is
        // settled as confirmed instead, since no further readback will run.
        function settleUnverified(previous) {
            if (previous?.generation !== generation) return previous;
            if (previous.outcome === 'provisional-confirmed' ||
                previous.outcome === 'final-confirmed') {
                return { ...previous, outcome: 'final-confirmed' };
            }
            if (previous.outcome === 'failed') return previous;

            return {
                ...target,
                generation,
                outcome: 'unverified',
                error: active.writeError?.message,
                startNowPlayingRevision: nowPlayingRevision,
            };
        }

        readbackDelays.forEach((delay, index) => {
            const timer = setTimeout(async () => {
                if (commandRef.current.active !== active) return;
                active.latestReadback = index;

                try {
                    const response = await api.deviceNowPlaying(deviceId);
                    if (commandRef.current.active !== active || active.latestReadback !== index) return;

                    const readbackStatus = response?.data?.status;
                    const nowPlaying = readbackStatus?.nowPlaying;
                    const readbackRevision = readbackStatus?.nowPlayingRevision;
                    const revisionIsNewer = nowPlayingRevision !== null &&
                        Number.isSafeInteger(readbackRevision) &&
                        readbackRevision > nowPlayingRevision;
                    onStatusReadback?.(readbackStatus);
                    if (revisionIsNewer && isErrorSource(nowPlaying?.Source)) {
                        clearReadbacks();
                        commandRef.current.active = null;
                        setCommand({
                            ...target,
                            generation,
                            outcome: 'failed',
                            error: nowPlaying.Source,
                            startNowPlayingRevision: nowPlayingRevision,
                        });
                    } else if (revisionIsNewer && nowPlaying?.Source === target.source &&
                        sourceAccountsMatch(target.source, nowPlaying?.SourceAccount, target.account)) {
                        // A match is not the end of the story: /select answers
                        // 200 even for a source the speaker goes on to reject a
                        // few seconds later, which surfaces as a transition to
                        // an error source. Something has to keep watching.
                        //
                        // The event stream is the better watcher when it is
                        // live: nowPlayingUpdated reports that transition as it
                        // happens, and the effect above already turns it into a
                        // failure. Polling on top of that only re-asks a
                        // question we are already subscribed to the answer of.
                        // So keep the remaining readbacks only as a fallback
                        // for a device whose events we are not receiving.
                        const isFinalReadback = index === readbackDelays.length - 1;
                        const eventStreamWatching = readbackStatus?.webSocketConnected === true;
                        const settled = isFinalReadback || eventStreamWatching;
                        setCommand({
                            ...target,
                            generation,
                            outcome: settled ? 'final-confirmed' : 'provisional-confirmed',
                            startNowPlayingRevision: nowPlayingRevision,
                            confirmedRevision: readbackRevision,
                        });
                        if (settled) {
                            clearReadbacks();
                            commandRef.current.active = null;
                        }
                    } else if (index === readbackDelays.length - 1) {
                        commandRef.current.active = null;
                        setCommand(settleUnverified);
                    }
                } catch (_) {
                    if (commandRef.current.active === active && active.latestReadback === index &&
                        index === readbackDelays.length - 1) {
                        commandRef.current.active = null;
                        setCommand(settleUnverified);
                    }
                }
            }, Math.max(0, delay - (Date.now() - startedAt)));
            commandRef.current.timers.push(timer);
        });

        try {
            await write();
        } catch (error) {
            if (commandRef.current.active !== active) return;
            // A definitive refusal (4xx) means the speaker never saw the
            // command, so there is nothing for the readbacks to confirm and
            // reporting it now beats waiting out the readback window. Anything
            // else stays pending: a 5xx or a transport error does not tell us
            // whether the speaker acted, so we keep verifying and carry the
            // reason into whatever outcome the readbacks reach.
            if (!error?.definitive) {
                active.writeError = error;

                return;
            }
            clearReadbacks();
            commandRef.current.active = null;
            setCommand(previous => previous?.generation === generation
                ? { ...previous, outcome: 'failed', error: error?.message }
                : previous);
        }
    }

    const projectsTarget = command && (command.outcome === 'pending' ||
        command.outcome === 'provisional-confirmed' || command.outcome === 'final-confirmed');
    const projectedSource = projectsTarget
        ? command.source : currentSource;
    const projectedAccount = projectsTarget
        ? command.account : (currentAccount ?? '');

    const outcomeText = {
        pending: 'Selecting source',
        'provisional-confirmed': 'Source selected, confirming',
        'final-confirmed': 'Source selected',
        unverified: 'Source selection unverified',
        failed: 'Source selection failed',
    };

    function commandMessage(cmd) {
        if (!cmd) return '';
        const text = outcomeText[cmd.outcome];
        return cmd.error ? `${text}: ${cmd.error}` : text;
    }

    return html`
        <div class="sources-section">
            <h3 class="section-title">Sources</h3>
            <div class="source-list">
                ${ready.map(src => {
                    const account = src.SourceAccount ?? '';
                    const isTarget = command?.source === src.Source && command?.account === account;
                    const isActive = src.Source === projectedSource &&
                        sourceAccountsMatch(src.Source, account, projectedAccount);
                    const outcome = isTarget ? command.outcome : '';
                    return html`
                        <button
                            key=${src.Source + account}
                            class="source-btn ${isActive ? 'active' : ''} ${src.IsLocal ? 'local' : ''} ${outcome}"
                            onClick=${() => select(src)}
                            disabled=${sourcesStale}
                            title=${availabilityMessage || (outcome ? commandMessage(command) : src.Source)}
                            aria-describedby=${availabilityId}
                            aria-busy=${outcome === 'pending' || outcome === 'provisional-confirmed' ? 'true' : null}
                        >
                            <span class="source-icon">${SOURCE_ICONS[src.Source] || '🔊'}</span>
                            <span class="source-name">${src.DisplayName || src.Source}</span>
                        </button>
                    `;
                })}
            </div>
            <div
                id=${availabilityId}
                class="source-command-status ${availabilityMessage ? 'availability' : ''}"
                role="status"
                aria-live="polite"
            >
                ${availabilityMessage || commandMessage(command)}
            </div>
        </div>
    `;
}
